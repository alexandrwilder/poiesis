package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Capture records the camera and microphone straight to a file with ffmpeg, per OS,
// at the calibrated settings: 720p, 30 fps, H.264 (see recordVideoArgs), AAC 128k mono.
// The audio level is read from ffmpeg's own stats so the record screen can show a bar.

type capture struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	out     string // "" in preview-only mode
	started time.Time
	mu      sync.Mutex
	levelDB float64 // RMS level in dB, about -60 (silence) to 0 (loud)
	done    chan error
	refl    *reflection // the faint self-view, or nil
}

// captureOptions: record to a file, and/or emit the preview stream for the reflection.
type captureOptions struct {
	Record     bool
	Out        string // recording path; "" = <vault>/inbox/<stamp>.mp4
	PreviewW   int    // 0 = no preview
	PreviewH   int
	PreviewFPS int // frames a second for the preview; 0 = 12
}

// concatSegments joins the parts of a paused entry into one clip without re-encoding.
func concatSegments(ffmpeg string, parts []string, out string) error {
	if len(parts) == 1 {
		return os.Rename(parts[0], out)
	}
	list := out + ".parts.txt"
	var b strings.Builder
	for _, p := range parts {
		b.WriteString("file '" + strings.ReplaceAll(p, "'", "'\\''") + "'\n")
	}
	if err := os.WriteFile(list, []byte(b.String()), 0o644); err != nil {
		return err
	}
	defer os.Remove(list)
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "concat", "-safe", "0", "-i", list, "-c", "copy", "-movflags", "+faststart", "-y", out)
	if o, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("joining the parts: %s: %w", strings.TrimSpace(string(o)), err)
	}
	for _, p := range parts {
		_ = os.Remove(p)
	}
	return nil
}

func defaultCaptureDevice() string {
	switch runtime.GOOS {
	case "darwin":
		return "0:0" // first camera, first microphone (ffmpeg -f avfoundation -list_devices true -i "")
	case "linux":
		return "/dev/video0:default" // v4l2 device : pulse/pipewire source
	case "windows":
		return `video=Integrated Camera:audio=Microphone`
	}
	return "0:0"
}

// recordVideoArgs is how the recording is encoded. On a Mac the hardware encoder does it:
// with the camera, ffmpeg took 38% of a core with x264 and 12% with the hardware encoder.
// It needs about 1.5 times the bytes of x264 for nearly the same picture (SSIM 0.981
// against 0.984 on a near-lossless camera reference), so it gets a fixed rate, which also
// keeps an entry's size predictable. Elsewhere x264 stays.
func recordVideoArgs() []string {
	if runtime.GOOS == "darwin" {
		return []string{"-c:v", "h264_videotoolbox", "-b:v", "1000k", "-realtime", "1"}
	}
	return []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p"}
}

func captureInputArgs(device string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"-f", "avfoundation", "-framerate", "30", "-video_size", "1280x720", "-pixel_format", "nv12", "-i", device}
	case "linux":
		video, audio := device, "default"
		if i := strings.LastIndex(device, ":"); i > 0 {
			video, audio = device[:i], device[i+1:]
		}
		return []string{"-f", "v4l2", "-framerate", "30", "-video_size", "1280x720", "-i", video, "-f", "pulse", "-i", audio}
	case "windows":
		return []string{"-f", "dshow", "-framerate", "30", "-video_size", "1280x720", "-i", device}
	}
	return []string{"-i", device}
}

var rmsRe = regexp.MustCompile(`RMS_level=(-?[0-9.]+)`)

// startCapture launches one ffmpeg process: recording to <vault>/inbox/<stamp>.mp4, the
// preview stream for the reflection, or both. The camera is opened once, by this process.
func startCapture(v *Vault, opts captureOptions) (*capture, error) {
	ffmpeg := findTool(v.Config.FFmpegBin)
	if _, err := exec.LookPath(ffmpeg); err != nil {
		return nil, fmt.Errorf("ffmpeg not found (%s): install it, or set ffmpeg_bin in config.json", v.Config.FFmpegBin)
	}
	args := []string{"-hide_banner", "-loglevel", "info", "-nostats"}
	args = append(args, captureInputArgs(v.Config.CaptureDevice)...)
	levelFilter := "astats=metadata=1:reset=1,ametadata=mode=print:key=lavfi.astats.Overall.RMS_level:direct=1"
	var refl *reflection
	if opts.PreviewW > 0 && opts.PreviewH > 0 {
		refl = &reflection{w: opts.PreviewW, h: opts.PreviewH}
		fps := opts.PreviewFPS
		if fps <= 0 {
			fps = 12
		}
		args = append(args, reflectionArgs(opts.PreviewW, opts.PreviewH, fps)...)
	}
	out := ""
	if opts.Record {
		out = opts.Out
		if out == "" {
			out = v.Path("inbox", time.Now().Format("2006-01-02T15-04-05")+".mp4")
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return nil, err
		}
		args = append(args, "-map", "0:v", "-map", "0:a", "-af", levelFilter)
		args = append(args, recordVideoArgs()...)
		args = append(args, "-c:a", "aac", "-b:a", "128k", "-ac", "1", "-movflags", "+faststart", "-y", out)
	} else {
		// preview only: keep the level bar alive by running the audio through a null output
		args = append(args, "-map", "0:a", "-af", levelFilter, "-f", "null", "-")
	}
	cmd := exec.Command(ffmpeg, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	var stdout io.ReadCloser
	if refl != nil {
		if stdout, err = cmd.StdoutPipe(); err != nil {
			return nil, err
		}
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}
	c := &capture{cmd: cmd, stdin: stdin, out: out, levelDB: -60, done: make(chan error, 1), refl: refl}
	if refl != nil {
		go refl.readFrames(stdout)
	}
	go func() {
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		var tail []string
		for sc.Scan() {
			line := sc.Text()
			if mm := rmsRe.FindStringSubmatch(line); mm != nil {
				if f, err := strconv.ParseFloat(mm[1], 64); err == nil {
					c.mu.Lock()
					c.levelDB = f
					if c.started.IsZero() {
						c.started = time.Now() // the clock starts with the first sound, not the launch
					}
					c.mu.Unlock()
				}
				continue
			}
			tail = append(tail, line)
			if len(tail) > 30 {
				tail = tail[1:]
			}
		}
		err := cmd.Wait()
		if err != nil {
			err = fmt.Errorf("%w: %s", err, strings.Join(tail, " | "))
		}
		c.done <- err
	}()
	return c, nil
}

// picture is the preview's frames, which the core draws itself on this path.
func (c *capture) picture() *reflection { return c.refl }

// ended receives when ffmpeg stops on its own.
func (c *capture) ended() <-chan error { return c.done }

// level returns the current audio level as 0..1 for a bar: a quiet room measures around
// -70 dB RMS, normal speech around -30 to -20, so the bar runs from -60 (empty) to -10 (full).
func (c *capture) level() float64 {
	c.mu.Lock()
	db := c.levelDB
	c.mu.Unlock()
	if db < -60 {
		db = -60
	}
	if db > -10 {
		db = -10
	}
	return (db + 60) / 50
}

// stop asks ffmpeg to finish cleanly (it reads 'q' on stdin on every OS) and waits.
func (c *capture) stop() error {
	fmt.Fprint(c.stdin, "q\n")
	c.stdin.Close()
	select {
	case err := <-c.done:
		return err
	case <-time.After(15 * time.Second):
		_ = c.cmd.Process.Kill()
		return fmt.Errorf("ffmpeg did not stop in time; file may be incomplete")
	}
}

// elapsed is the time recorded so far: zero until the first sound has arrived.
func (c *capture) elapsed() time.Duration {
	c.mu.Lock()
	st := c.started
	c.mu.Unlock()
	if st.IsZero() {
		return 0
	}
	return time.Since(st)
}

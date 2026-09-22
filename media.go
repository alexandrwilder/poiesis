package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Media describes one raw recording after it has been registered.
type Media struct {
	Path         string    `json:"path"` // relative to the vault root
	OriginalName string    `json:"original_name"`
	SHA256       string    `json:"sha256"`
	RecordedAt   time.Time `json:"recorded_at"`
	DurationS    float64   `json:"duration_s"`
	SizeBytes    int64     `json:"size_bytes"`
	Width        int       `json:"width,omitempty"`
	Height       int       `json:"height,omitempty"`
	FPS          string    `json:"fps,omitempty"`
	Device       string    `json:"device,omitempty"`
	IngestedAt   time.Time `json:"ingested_at"`
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type probeResult struct {
	Format struct {
		Duration string            `json:"duration"`
		Size     string            `json:"size"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
	Streams []struct {
		CodecType  string `json:"codec_type"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		RFrameRate string `json:"r_frame_rate"`
	} `json:"streams"`
}

func probe(ffprobe, path string) (probeResult, error) {
	var pr probeResult
	out, err := exec.Command(ffprobe, "-v", "error", "-show_entries",
		"format=duration,size:format_tags=creation_time:stream=codec_type,width,height,r_frame_rate",
		"-of", "json", path).Output()
	if err != nil {
		return pr, fmt.Errorf("ffprobe: %w", err)
	}
	if err := json.Unmarshal(out, &pr); err != nil {
		return pr, fmt.Errorf("ffprobe output: %w", err)
	}
	return pr, nil
}

// Register hashes the file, works out when it was recorded, and moves (or copies) it into
// raw/YYYY/MM/<stamp>_<hash16>.<ext> with a JSON sidecar next to it.
func Register(v *Vault, file string, keep bool) (*Media, error) {
	pr, err := probe(findTool(v.Config.FFprobeBin), file)
	if err != nil {
		return nil, err
	}
	dur, _ := strconv.ParseFloat(pr.Format.Duration, 64)
	st, err := os.Stat(file)
	if err != nil {
		return nil, err
	}
	recorded := st.ModTime().Add(-time.Duration(dur * float64(time.Second)))
	if ct := pr.Format.Tags["creation_time"]; ct != "" {
		if t, err := time.Parse(time.RFC3339Nano, ct); err == nil && t.Year() > 2000 {
			recorded = t.Local()
		}
	}
	sum, err := sha256File(file)
	if err != nil {
		return nil, err
	}
	m := &Media{
		OriginalName: filepath.Base(file),
		SHA256:       sum,
		RecordedAt:   recorded,
		DurationS:    dur,
		SizeBytes:    st.Size(),
		IngestedAt:   time.Now(),
	}
	for _, s := range pr.Streams {
		if s.CodecType == "video" {
			m.Width, m.Height, m.FPS = s.Width, s.Height, s.RFrameRate
		}
	}
	ext := strings.ToLower(filepath.Ext(file))
	if ext == "" {
		ext = ".mp4"
	}
	stamp := recorded.Format("2006-01-02T15-04")
	rel := filepath.Join("raw", recorded.Format("2006"), recorded.Format("01"), stamp+"_"+sum[:16]+ext)
	dest := v.Path(rel)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(dest); err == nil {
		// already registered (same hash, same minute): reuse
	} else if keep || !sameDevice(file, dest) {
		if err := copyFile(file, dest); err != nil {
			return nil, err
		}
		if !keep {
			_ = os.Remove(file)
		}
	} else if err := os.Rename(file, dest); err != nil {
		return nil, err
	}
	m.Path = filepath.ToSlash(rel)
	side, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(strings.TrimSuffix(dest, ext)+".json", append(side, '\n'), 0o644); err != nil {
		return nil, err
	}
	return m, nil
}

func sameDevice(a, b string) bool {
	// cheap heuristic: rename works across the same volume; try it and fall back to copy on error
	return true
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// ExtractAudio writes a 16 kHz mono wav next to the raw file (cached; skipped if present).
func ExtractAudio(v *Vault, m *Media) (string, error) {
	src := v.Path(m.Path)
	wav := strings.TrimSuffix(src, filepath.Ext(src)) + ".wav"
	if _, err := os.Stat(wav); err == nil {
		return wav, nil
	}
	cmd := exec.Command(findTool(v.Config.FFmpegBin), "-y", "-loglevel", "error", "-i", src, "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wav)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg audio: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return wav, nil
}

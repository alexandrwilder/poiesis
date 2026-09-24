package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// An entry cut short by a quit or a crash leaves its parts in inbox/.parts. The next start
// joins them into one clip to be processed; a part no program can read is kept aside.
func TestAnUnfinishedEntryIsJoinedAndKept(t *testing.T) {
	ffmpeg, ffprobe := findTool("ffmpeg"), findTool("ffprobe")
	if _, err := exec.LookPath(ffmpeg); err != nil {
		t.Skip("needs ffmpeg")
	}
	v, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parts := v.Path("inbox", ".parts")
	if err := os.MkdirAll(parts, 0o755); err != nil {
		t.Fatal(err)
	}
	// the first entry in two parts, a second entry in one, and a third that was never closed
	for _, name := range []string{"2026-09-24T10-00-00.part01.mp4", "2026-09-24T10-01-00.part02.mp4", "2026-09-24T10-03-00.part01.mp4"} {
		out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "testsrc=size=160x90:rate=10", "-f", "lavfi", "-i", "sine=frequency=440",
			"-t", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", filepath.Join(parts, name)).CombinedOutput()
		if err != nil {
			t.Skipf("this ffmpeg cannot make a test clip: %s", out)
		}
	}
	broken := "2026-09-24T10-05-00.part01.mp4"
	if err := os.WriteFile(filepath.Join(parts, broken), []byte("not a video"), 0o644); err != nil {
		t.Fatal(err)
	}

	waiting, err := v.waitingRecordings()
	if err != nil {
		t.Fatal(err)
	}
	joined, second := v.Path("inbox", "2026-09-24T10-00-00.mp4"), v.Path("inbox", "2026-09-24T10-03-00.mp4")
	if len(waiting) != 2 || waiting[0].file != joined || waiting[1].file != second || waiting[0].inRaw {
		t.Fatalf("waiting: %+v", waiting)
	}
	pr, err := probe(ffprobe, joined)
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := strconv.ParseFloat(pr.Format.Duration, 64); d < 1.8 {
		t.Fatalf("the joined clip is %.2f s; both parts should be in it", d)
	}
	if _, err := os.Stat(filepath.Join(parts, "unreadable", broken)); err != nil {
		t.Fatalf("the unreadable part was not kept: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(parts, "*.mp4")); len(left) != 0 {
		t.Fatalf("parts left behind: %v", left)
	}
}

// The window takes up what was waiting when it opens: processed one at a time, the picture
// resting meanwhile.
func TestTheWindowProcessesWhatWasWaiting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	opened := countCameraOpens(t)
	v, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v.Path("inbox", "2026-09-24T09-00-00.mp4"), []byte("a clip"), 0o644); err != nil {
		t.Fatal(err)
	}
	v.Config.Reflection = "on"
	m := &tuiModel{v: v, data: &tuiData{}, focused: true}
	m.Init()
	if m.record.pending != 1 || !m.record.processing {
		t.Fatalf("pending %d, processing %v", m.record.pending, m.record.processing)
	}
	if *opened != 0 {
		t.Fatalf("the picture started while a waiting entry is processed")
	}
}

// A recording whose camera stops on its own keeps what it has and says so, instead of a clock
// counting over nothing.
func TestARecordingThatStopsOnItsOwnSaysSo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	countCameraOpens(t)
	m := &tuiModel{v: &Vault{Config: defaultConfig()}, data: &tuiData{}}
	m.v.Config.Reflection = "off"
	cam := &fakeCamera{el: 40 * time.Second, end: make(chan error, 1)}
	m.scr, m.record.phase, m.record.cap = screenRecord, "recording", cam
	cam.end <- errors.New("the camera was unplugged")
	m.Update(tickMsg(time.Now()))
	if m.record.phase != "paused" || m.record.cap != nil || m.record.recorded != 40*time.Second {
		t.Fatalf("phase %q, camera %v, recorded %v", m.record.phase, m.record.cap, m.record.recorded)
	}
	if !strings.Contains(m.record.lastErr, "the recording stopped") {
		t.Fatalf("nothing was said: %q", m.record.lastErr)
	}
}

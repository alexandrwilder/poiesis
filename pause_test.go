package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A paused entry is two parts joined into one clip. The joined clip must be about as long
// as the parts together and carry one video and one audio stream. POIESIS_CAPTURE_TEST=1 to run.
func TestPauseJoin(t *testing.T) {
	if os.Getenv("POIESIS_CAPTURE_TEST") != "1" {
		t.Skip("needs a camera; run with POIESIS_CAPTURE_TEST=1")
	}
	dir := t.TempDir()
	v, err := OpenVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for i := 1; i <= 2; i++ {
		out := filepath.Join(dir, "inbox", ".parts", "p"+strconv.Itoa(i)+".mp4")
		c, err := startCapture(v, captureOptions{Record: true, Out: out})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Second)
		if err := c.stop(); err != nil {
			t.Fatal(err)
		}
		parts = append(parts, out)
	}
	joined := filepath.Join(dir, "inbox", "joined.mp4")
	if err := concatSegments(v.Config.FFmpegBin, parts, joined); err != nil {
		t.Fatal(err)
	}
	for _, p := range parts {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("part not removed: %s", p)
		}
	}
	probe := func(f string) (float64, int) {
		o, err := exec.Command(v.Config.FFprobeBin, "-v", "error", "-show_entries", "format=duration:stream=codec_type", "-of", "default=nw=1", f).Output()
		if err != nil {
			t.Fatal(err)
		}
		d, n := 0.0, 0
		for _, l := range strings.Split(string(o), "\n") {
			if strings.HasPrefix(l, "duration=") {
				d, _ = strconv.ParseFloat(strings.TrimPrefix(l, "duration="), 64)
			}
			if strings.HasPrefix(l, "codec_type=") {
				n++
			}
		}
		return d, n
	}
	d, n := probe(joined)
	if n != 2 {
		t.Errorf("joined clip has %d streams, want 2", n)
	}
	if d < 3.0 || d > 5.5 {
		t.Errorf("joined clip is %.1fs, want about 4s", d)
	}
	t.Logf("joined: %.1fs, %d streams", d, n)
}

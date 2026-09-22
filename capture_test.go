package main

import (
	"os"
	"testing"
	"time"
)

// TestCapture records three seconds from the real camera and microphone. It only runs
// when POIESIS_CAPTURE_TEST=1 because it needs a camera and asks macOS for permission.
func TestCapture(t *testing.T) {
	if os.Getenv("POIESIS_CAPTURE_TEST") == "" {
		t.Skip("set POIESIS_CAPTURE_TEST=1 to record three seconds from the camera")
	}
	dir := t.TempDir()
	v, err := OpenVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, err := startCapture(v, captureOptions{Record: true, PreviewW: 80, PreviewH: 44})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	level := c.level()
	if f := c.refl.snapshot(); f == nil {
		t.Errorf("no preview frame arrived in 3 s")
	} else {
		t.Logf("preview frames received: %d, first row of the render: %q", c.refl.got, renderReflection(f, 80, 44, 80, 44/2, "faint", nil)[:60])
	}
	if err := c.stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	st, err := os.Stat(c.out)
	if err != nil {
		t.Fatalf("no output file: %v", err)
	}
	if st.Size() < 50_000 {
		t.Fatalf("output too small: %d bytes", st.Size())
	}
	pr, err := probe(v.Config.FFprobeBin, c.out)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("recorded %s: %d bytes, duration %s, audio level while recording %.2f, streams %d", c.out, st.Size(), pr.Format.Duration, level, len(pr.Streams))
	if len(pr.Streams) < 2 {
		t.Fatalf("expected video and audio streams, got %d", len(pr.Streams))
	}
}

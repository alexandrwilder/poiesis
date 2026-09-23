package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The recording's encoder settings must actually encode with this system's ffmpeg: two
// seconds of a camera-like picture and a tone, with the same video and audio arguments
// the app records with, must give an H.264 file with both streams.
func TestRecordVideoArgsEncode(t *testing.T) {
	ff, err := exec.LookPath(findTool("ffmpeg"))
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	out := filepath.Join(t.TempDir(), "entry.mp4")
	args := []string{"-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30,format=nv12",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "2", "-map", "0:v", "-map", "1:a"}
	args = append(args, recordVideoArgs()...)
	args = append(args, "-c:a", "aac", "-b:a", "128k", "-ac", "1", "-movflags", "+faststart", out)
	if b, err := exec.Command(ff, args...).CombinedOutput(); err != nil {
		t.Fatalf("encoding with %v failed: %v\n%s", recordVideoArgs(), err, b)
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		t.Fatalf("no file written: %v", err)
	}
	probe, err := exec.LookPath(findTool("ffprobe"))
	if err != nil {
		t.Skip("ffprobe not installed")
	}
	b, err := exec.Command(probe, "-v", "error", "-show_entries", "stream=codec_name", "-of", "csv=p=0", out).Output()
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(b))
	if len(got) != 2 || got[0] != "h264" || got[1] != "aac" {
		t.Fatalf("streams %v, want h264 and aac", got)
	}
}

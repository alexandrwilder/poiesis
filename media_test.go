package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A recording whose audio has a hole in its timeline (the Mac camera input drops bits of
// sound) must still give a wav as long as the recording, so every word keeps its second.
func TestExtractAudioKeepsTheTimeline(t *testing.T) {
	ff, err := exec.LookPath(findTool("ffmpeg"))
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "raw"), 0o755); err != nil {
		t.Fatal(err)
	}
	clip := filepath.Join(root, "raw", "gap.mp4")
	// six seconds of tone with the second between 2 and 3 missing, timestamps kept
	mk := exec.Command(ff, "-v", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=6",
		"-af", "aselect='not(between(t,2,3))'", "-c:a", "aac", clip)
	if out, err := mk.CombinedOutput(); err != nil {
		t.Fatalf("making the clip: %v\n%s", err, out)
	}
	v := &Vault{Root: root, Config: defaultConfig()}
	wav, err := ExtractAudio(v, &Media{Path: filepath.Join("raw", "gap.mp4")})
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(wav)
	if err != nil {
		t.Fatal(err)
	}
	secs := float64(st.Size()-44) / 32000 // 16 kHz mono 16-bit
	if secs < 5.8 || secs > 6.2 {
		t.Fatalf("the wav is %.2f s; the recording is 6 s, so words after the hole would sit at the wrong second", secs)
	}
}

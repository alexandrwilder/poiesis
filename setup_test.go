package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The download path of setup, on the smallest model (under 1 MB): a fresh folder ends up
// with the file, the checksum holds, and a second run downloads nothing. POIESIS_NET_TEST=1.
func TestEnsureModelDownloads(t *testing.T) {
	if os.Getenv("POIESIS_NET_TEST") != "1" {
		t.Skip("needs the network; run with POIESIS_NET_TEST=1")
	}
	dir := t.TempDir()
	var lines []string
	say := func(f string, a ...any) { lines = append(lines, f) }
	vad := speechModels[1]
	if err := ensureModel(dir, vad, say); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dir, vad.Name))
	if err != nil || st.Size() != vad.Size {
		t.Fatalf("file missing or wrong size: %v", err)
	}
	n := len(lines)
	if err := ensureModel(dir, vad, say); err != nil {
		t.Fatal(err)
	}
	if len(lines) != n+1 || lines[n] != "  ✓ %-32s %8s  ·  %s" {
		t.Errorf("second run should only report the file as present, got %v", lines[n:])
	}
	// a corrupted file is noticed and re-downloaded
	if err := os.WriteFile(filepath.Join(dir, vad.Name), make([]byte, vad.Size), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureModel(dir, vad, say); err != nil {
		t.Fatal(err)
	}
	if sum, _ := fileSHA256(filepath.Join(dir, vad.Name)); sum != vad.SHA {
		t.Errorf("corrupted file was not replaced")
	}
}

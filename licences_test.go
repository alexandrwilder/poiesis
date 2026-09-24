package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every program and library inside the app brings its licence text with it, and a package
// without one is named, so a release never ships a program without its licence unnoticed.
func TestTheAppCarriesTheLicences(t *testing.T) {
	root := t.TempDir()
	put := func(rel, body string) string {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ffmpeg := put("Cellar/ffmpeg/9.0.2/bin/ffmpeg", "a program")
	put("Cellar/ffmpeg/9.0.2/COPYING.GPLv2", "GPL text")
	put("Cellar/ffmpeg/9.0.2/LICENSE.md", "ffmpeg licence")
	x264 := put("Cellar/x264/r3222/lib/libx264.dylib", "a library")
	put("Cellar/x264/r3222/COPYING", "x264 GPL text")
	bare := put("Cellar/bare/1.0/lib/libbare.dylib", "a library without a licence")

	app := filepath.Join(root, "Poiesis.app")
	var said []string
	copied := map[string]string{ffmpeg: "", x264: "", bare: ""}
	if err := bundleLicences(app, copied, func(f string, a ...any) { said = append(said, fmt.Sprintf(f, a...)) }); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"ffmpeg-9.0.2/COPYING.GPLv2", "ffmpeg-9.0.2/LICENSE.md", "x264-r3222/COPYING"} {
		if _, err := os.Stat(filepath.Join(app, "Contents", "Resources", "licenses", rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
	if !strings.Contains(strings.Join(said, "\n"), "bare-1.0") {
		t.Errorf("the package without a licence was not named: %q", said)
	}
}

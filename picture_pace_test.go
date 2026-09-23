package main

import (
	"testing"
	"time"
)

// Twelve frames a second for the clear picture: every frame costs the window a copy, a
// colour conversion, a new texture and a full redraw, so the frame rate is the lever.
// The screen ticks at the same pace, so it never redraws between frames for nothing.
func TestPicturePace(t *testing.T) {
	cases := []struct {
		video    string
		fps      int
		interval time.Duration
	}{
		{"clear", 12, time.Second / 12},
		{"light", 10, time.Second / 10},
		{"styled", 10, time.Second / 10},
	}
	for _, c := range cases {
		m := &tuiModel{v: &Vault{Config: defaultConfig()}}
		m.v.Config.Video = c.video
		if got := m.previewFPS(); got != c.fps {
			t.Errorf("%s: %d frames a second, want %d", c.video, got, c.fps)
		}
		if got := m.frameInterval(); got != c.interval {
			t.Errorf("%s: a tick every %v, want %v", c.video, got, c.interval)
		}
	}
}

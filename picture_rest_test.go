package main

import "testing"

// Right after enter, the speech model and the local AI fill the graphics chip; a live
// picture drawn on the same chip made the app lag. The picture rests until the entry is done.
func TestPictureRestsWhileProcessing(t *testing.T) {
	m := &tuiModel{v: &Vault{Config: defaultConfig()}, focused: true}
	m.v.Config.Reflection = "on"
	if !m.pictureMayStart() {
		t.Fatal("with the camera on and nothing being processed, the picture may start")
	}
	m.record.processing = true
	if m.pictureMayStart() {
		t.Fatal("while an entry is processed the picture must rest")
	}
	m.record.processing = false
	m.v.Config.Reflection = "off"
	if m.pictureMayStart() {
		t.Fatal("with the camera turned off the picture must not start")
	}
}

package main

import "testing"

// Every theme must read on the patch behind text and over a bright picture: the text
// colours bright, the frame lines not too dark, and the look it names must exist.
func TestThemeContrast(t *testing.T) {
	for _, th := range themes {
		for _, c := range []struct{ name, hex string }{{"accent", th.Accent}, {"accent2", th.Accent2}, {"ink", th.Ink}, {"mid", th.Mid}} {
			if l := luminance(c.hex); l < 0.62 {
				t.Errorf("%s: %s %s is too dark to read on a picture (luminance %.2f)", th.Name, c.name, c.hex, l)
			}
		}
		if l := luminance(th.Dim); l < 0.45 {
			t.Errorf("%s: dim text %s too dark (%.2f)", th.Name, th.Dim, l)
		}
		if l := luminance(th.Rule); l < 0.35 {
			t.Errorf("%s: frame lines %s vanish over a picture (%.2f)", th.Name, th.Rule, l)
		}
		if th.Patch < 0.2 || th.Patch > 0.5 {
			t.Errorf("%s: patch %.2f outside 0.2..0.5", th.Name, th.Patch)
		}
		if lookByName(th.Look).Name != th.Look {
			t.Errorf("%s names a look that does not exist: %s", th.Name, th.Look)
		}
	}
}

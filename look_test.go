package main

import "testing"

// A look at zero strength changes nothing; "true" never changes anything; a stronger
// intensity moves a pixel further from the original than a softer one.
func TestLooks(t *testing.T) {
	r, g, b := uint8(200), uint8(120), uint8(60)
	if p := lookByName("true").pixel(1, r, g, b); p != [3]uint8{r, g, b} {
		t.Errorf("true changed a pixel: %v", p)
	}
	for _, l := range looks {
		if p := l.pixel(0, r, g, b); p != [3]uint8{r, g, b} {
			t.Errorf("%s at zero strength changed a pixel: %v", l.Name, p)
		}
		soft := l.pixel(0.45, r, g, b)
		strong := l.pixel(1, r, g, b)
		if l.Name != "true" && dist(soft, [3]uint8{r, g, b}) > dist(strong, [3]uint8{r, g, b})+1 {
			t.Errorf("%s: soft moved the pixel further than strong (%v vs %v)", l.Name, soft, strong)
		}
	}
	if m := lookByName("mono").pixel(1, r, g, b); !(m[0] == m[1] && m[1] == m[2]) {
		t.Errorf("mono is not grey: %v", m)
	}
	f := make([]byte, 6)
	copy(f, []byte{r, g, b, r, g, b})
	if out := applyLook(lookByName("night"), 1, f); out[0] >= r {
		t.Errorf("night did not darken: %v", out[:3])
	}
}

func dist(a, b [3]uint8) int {
	d := 0
	for i := range a {
		x := int(a[i]) - int(b[i])
		if x < 0 {
			x = -x
		}
		d += x
	}
	return d
}

package main

// The look of the camera picture: how the pixels are treated before they are shown, at a
// chosen strength. It applies to clear video (the pixels are changed before they go to the
// terminal) and to the styled picture alike. "true" shows the camera as it is.

type look struct {
	Name, Note string
	// at full strength; the intensity blends this with the untouched pixel
	f func(r, g, b float64) (float64, float64, float64)
}

var looks = []look{
	{"true", "the camera as it is", func(r, g, b float64) (float64, float64, float64) { return r, g, b }},
	{"faded", "washed and dim, so you look, not stare", func(r, g, b float64) (float64, float64, float64) {
		l := lum(r, g, b)
		return mix(l, r, 0.3)*0.5 + 12, mix(l, g, 0.3)*0.47 + 9, mix(l, b, 0.3)*0.42 + 6
	}},
	{"warm", "late light", func(r, g, b float64) (float64, float64, float64) { return clamp(r*1.08 + 6), g * 0.94, b * 0.72 }},
	{"cool", "early light", func(r, g, b float64) (float64, float64, float64) { return r * 0.78, g * 0.94, clamp(b*1.12 + 8) }},
	{"mono", "black and white", func(r, g, b float64) (float64, float64, float64) { l := lum(r, g, b); return l, l, l }},
	{"noir", "hard black and white", func(r, g, b float64) (float64, float64, float64) {
		l := clamp((lum(r, g, b)-128)*1.6 + 100)
		return l, l, l
	}},
	{"sepia", "old paper", func(r, g, b float64) (float64, float64, float64) {
		return clamp(0.393*r + 0.769*g + 0.189*b), clamp(0.349*r + 0.686*g + 0.168*b), clamp(0.272*r + 0.534*g + 0.131*b)
	}},
	{"night", "almost dark, a little blue", func(r, g, b float64) (float64, float64, float64) {
		l := lum(r, g, b)
		return l * 0.22, l * 0.26, l*0.36 + 10
	}},
	{"vivid", "colour turned up", func(r, g, b float64) (float64, float64, float64) {
		l := lum(r, g, b)
		return clamp(mix(l, r, 1.5) * 1.05), clamp(mix(l, g, 1.5) * 1.05), clamp(mix(l, b, 1.5) * 1.05)
	}},
}

func lum(r, g, b float64) float64 { return 0.299*r + 0.587*g + 0.114*b }

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// mix moves v away from the grey l by k (k<1 fades colour, k>1 boosts it).
func mix(l, v, k float64) float64 { return clamp(l + (v-l)*k) }

func lookNames() []string {
	var out []string
	for _, l := range looks {
		out = append(out, l.Name)
	}
	return out
}

func lookByName(name string) look {
	for _, l := range looks {
		if l.Name == name {
			return l
		}
	}
	return looks[0]
}

// pixel applies a look at a strength to one pixel.
func (l look) pixel(k float64, r, g, b uint8) [3]uint8 {
	if l.Name == "true" || k <= 0 {
		return [3]uint8{r, g, b}
	}
	fr, fg, fb := float64(r), float64(g), float64(b)
	tr, tg, tb := l.f(fr, fg, fb)
	return [3]uint8{uint8(clamp(fr + (tr-fr)*k)), uint8(clamp(fg + (tg-fg)*k)), uint8(clamp(fb + (tb-fb)*k))}
}

// applyLook returns the frame with the look applied (a copy), or the frame itself for "true".
var lookBuf []byte

func applyLook(l look, k float64, frame []byte) []byte {
	if l.Name == "true" || k <= 0 {
		return frame
	}
	if len(lookBuf) != len(frame) {
		lookBuf = make([]byte, len(frame))
	}
	out := lookBuf
	for i := 0; i+2 < len(frame); i += 3 {
		p := l.pixel(k, frame[i], frame[i+1], frame[i+2])
		out[i], out[i+1], out[i+2] = p[0], p[1], p[2]
	}
	return out
}

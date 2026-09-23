package main

// The look of the camera picture: how the pixels are treated before they are shown, at a
// chosen strength, for clear video and the styled picture alike. "true" shows the camera as
// it is. Every look is affine, a colour matrix clamped once at the end, so the terminal and
// every host (look.matrix, docs/HOST.md) show the same pixels.

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
	{"warm", "late light", func(r, g, b float64) (float64, float64, float64) { return r*1.08 + 6, g * 0.94, b * 0.72 }},
	{"cool", "early light", func(r, g, b float64) (float64, float64, float64) { return r * 0.78, g * 0.94, b*1.12 + 8 }},
	{"mono", "black and white", func(r, g, b float64) (float64, float64, float64) { l := lum(r, g, b); return l, l, l }},
	{"noir", "hard black and white", func(r, g, b float64) (float64, float64, float64) {
		l := (lum(r, g, b)-128)*1.6 + 100
		return l, l, l
	}},
	{"sepia", "old paper", func(r, g, b float64) (float64, float64, float64) {
		return 0.393*r + 0.769*g + 0.189*b, 0.349*r + 0.686*g + 0.168*b, 0.272*r + 0.534*g + 0.131*b
	}},
	{"night", "almost dark, a little blue", func(r, g, b float64) (float64, float64, float64) {
		l := lum(r, g, b)
		return l * 0.22, l * 0.26, l*0.36 + 10
	}},
	{"vivid", "colour turned up", func(r, g, b float64) (float64, float64, float64) {
		l := lum(r, g, b)
		return mix(l, r, 1.5) * 1.05, mix(l, g, 1.5) * 1.05, mix(l, b, 1.5) * 1.05
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

// mix moves v away from the grey l by k (k<1 fades colour, k>1 boosts it). No clamp here:
// a look is affine, and the one clamp comes last, in pixel and in every host.
func mix(l, v, k float64) float64 { return l + (v-l)*k }

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

// matrix is the look at strength k as a colour matrix, the form every graphics system can
// apply (Core Image, GTK, a web view): three rows for red, green and blue, each the weights of
// red, green and blue and an offset, on a 0..1 scale, the result clamped. Every look is
// affine, so four points measure it exactly. A host applies these twelve numbers and never
// needs to know a look by name (docs/HOST.md).
func (l look) matrix(k float64) [12]float64 {
	const mid, step = 128.0, 16.0
	at := func(r, g, b float64) [3]float64 {
		fr, fg, fb := l.f(r, g, b)
		return [3]float64{r + (fr-r)*k, g + (fg-g)*k, b + (fb-b)*k}
	}
	base := at(mid, mid, mid)
	dr, dg, db := at(mid+step, mid, mid), at(mid, mid+step, mid), at(mid, mid, mid+step)
	var m [12]float64
	for row := 0; row < 3; row++ {
		wr, wg, wb := (dr[row]-base[row])/step, (dg[row]-base[row])/step, (db[row]-base[row])/step
		m[row*4], m[row*4+1], m[row*4+2] = wr, wg, wb
		m[row*4+3] = (base[row] - (wr+wg+wb)*mid) / 255
	}
	return m
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

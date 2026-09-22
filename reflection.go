package main

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// The faint reflection: a second, low-resolution output of the same ffmpeg process that
// records (the camera can only be opened once), painted as half-block characters at a
// fifth of full brightness in one warm tint, with the readout drawn on top.
// It never touches the recorded file; the preview stream is scaled and dropped after drawing.

type reflection struct {
	w, h  int // pixels: w = terminal columns, h = 2 × rows
	mu    sync.Mutex
	frame []byte // latest rgb24 frame, len w*h*3, or nil
	got   int
}

// reflectionArgs are the extra ffmpeg output options that emit the preview to stdout.
func reflectionArgs(w, h, fps int) []string {
	return []string{"-map", "0:v", "-vf", fmt.Sprintf("fps=%d,scale=%d:%d", fps, w, h), "-f", "rawvideo", "-pix_fmt", "rgb24", "pipe:1"}
}

// frames counts the frames received so far.
func (r *reflection) frames() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.got
}

// readFrames keeps the newest frame from the pipe until it closes.
func (r *reflection) readFrames(pipe io.Reader) {
	size := r.w * r.h * 3
	buf := make([]byte, size)
	for {
		if _, err := io.ReadFull(pipe, buf); err != nil {
			return
		}
		r.mu.Lock()
		if r.frame == nil {
			r.frame = make([]byte, size)
		}
		copy(r.frame, buf)
		r.got++
		r.mu.Unlock()
	}
}

func (r *reflection) snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frame == nil {
		return nil
	}
	out := make([]byte, len(r.frame))
	copy(out, r.frame)
	return out
}

// overlayText is a piece of readout to draw on top of the reflection.
type overlayText struct {
	row, col int
	text     string
	rgb      [3]uint8
	bg       float64 // how much of the picture shows behind the text: 0 means 0.4; 1 keeps it all
}

var (
	rgbInk     = [3]uint8{230, 228, 220}
	rgbMid     = [3]uint8{167, 170, 176}
	rgbDim     = [3]uint8{111, 116, 124}
	rgbEnd     = [3]uint8{0xE0, 0x6C, 0x4E} // the last half minute of an entry
	rgbAmber   = [3]uint8{232, 163, 61}
	rgbOK      = [3]uint8{125, 190, 138}
	rgbRule    = [3]uint8{0x3A, 0x3F, 0x47}
	rgbAccent2 = [3]uint8{232, 163, 61}
)

// renderReflection draws the frame as rows of half-blocks (top pixel = foreground,
// bottom pixel = background) and splices the overlay text in. mode: faint | clear.
// renderReflection paints the newest camera frame as half-block characters and draws the
// readout on top. The frame is sampled to whatever size the window is now, so dragging the
// window keeps the picture filling it without interrupting a recording.
func renderReflection(frame []byte, sw, sh, tw, th int, mode string, overlays []overlayText) string {
	w, rows := tw, th
	if w < 1 || rows < 1 || sw < 1 || sh < 1 {
		return ""
	}
	// pixel → colour in the chosen look, sampled from the part of the frame that has the
	// window's shape (never stretched)
	style := pictureStyle(mode)
	cx, cy, cw, chh := cropRect(sw, sh, w, rows)
	shade := func(tx, ty int) [3]uint8 {
		x := cx + tx*cw/w
		y := cy + ty*chh/(rows*2)
		i := (y*sw + x) * 3
		if i+2 >= len(frame) {
			return [3]uint8{0, 0, 0}
		}
		return styleColor(style, float64(frame[i]), float64(frame[i+1]), float64(frame[i+2]))
	}
	// index overlays by row
	byRow := map[int][]overlayText{}
	for _, o := range overlays {
		byRow[o.row] = append(byRow[o.row], o)
	}
	var b strings.Builder
	for y := 0; y < rows; y++ {
		type cell struct {
			ch  rune
			fg  [3]uint8
			set bool
			bg  float64
		}
		cells := make([]cell, w)
		for _, o := range byRow[y] {
			mul := o.bg
			if mul == 0 {
				mul = textPatch
			}
			for i, ch := range []rune(o.text) {
				x := o.col + i
				if x >= 0 && x < w {
					cells[x] = cell{ch: ch, fg: o.rgb, set: true, bg: mul}
				}
			}
		}
		for x := 0; x < w; x++ {
			top, bot := shade(x, 2*y), shade(x, 2*y+1)
			if cells[x].set {
				// text sits on the picture, taken down a little so it reads; frame lines keep it
				k := cells[x].bg / 2
				bg := [3]uint8{uint8(float64(int(top[0])+int(bot[0])) * k), uint8(float64(int(top[1])+int(bot[1])) * k), uint8(float64(int(top[2])+int(bot[2])) * k)}
				fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm%c", cells[x].fg[0], cells[x].fg[1], cells[x].fg[2], bg[0], bg[1], bg[2], cells[x].ch)
				continue
			}
			fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", top[0], top[1], top[2], bot[0], bot[1], bot[2])
		}
		b.WriteString("\x1b[0m")
		if y < rows-1 {
			b.WriteString("\n") // exactly the window's rows: one more would scroll the top away
		}
	}
	return b.String()
}

// styleColor is one camera pixel with the chosen look at its strength.
func styleColor(_ string, rr, gg, bb float64) [3]uint8 {
	return currentLook.pixel(currentStrength, uint8(rr), uint8(gg), uint8(bb))
}

var currentLook = looks[0]
var currentStrength = 0.0

// edgeColor is the average of the picture's top rows in the chosen look: the colour the
// window's title bar takes, so the picture seems to continue under it.
func edgeColor(frame []byte, sw, sh int, mode string) [3]uint8 {
	if sw < 1 || sh < 1 || len(frame) < sw*3 {
		return [3]uint8{0, 0, 0}
	}
	rowsUsed := sh / 12
	if rowsUsed < 1 {
		rowsUsed = 1
	}
	var r, g, b, n float64
	for y := 0; y < rowsUsed; y++ {
		for x := 0; x < sw; x++ {
			i := (y*sw + x) * 3
			if i+2 >= len(frame) {
				break
			}
			r += float64(frame[i])
			g += float64(frame[i+1])
			b += float64(frame[i+2])
			n++
		}
	}
	if n == 0 {
		return [3]uint8{0, 0, 0}
	}
	return styleColor(pictureStyle(mode), r/n, g/n, b/n)
}

// windowTint asks the terminal to paint its background (and so the title bar) this colour.
func windowTint(c [3]uint8) string { return fmt.Sprintf("\x1b]11;#%02x%02x%02x\x07", c[0], c[1], c[2]) }

// windowTintReset gives the terminal its own background back.
const windowTintReset = "\x1b]111\x07"

// renderTextOver draws only the overlays, on transparent rows, for clear video: the
// terminal shows the picture wherever nothing is written. Text gets a dark patch.
func renderTextOver(w, h int, overlays []overlayText) string {
	byRow := map[int][]overlayText{}
	for _, o := range overlays {
		byRow[o.row] = append(byRow[o.row], o)
	}
	var b strings.Builder
	for y := 0; y < h; y++ {
		type cell struct {
			ch    rune
			fg    [3]uint8
			set   bool
			patch bool
		}
		cells := make([]cell, w)
		for _, o := range byRow[y] {
			for i, ch := range []rune(o.text) {
				x := o.col + i
				if x >= 0 && x < w {
					cells[x] = cell{ch: ch, fg: o.rgb, set: true, patch: o.bg < 0.6 && o.bg != 0.9}
				}
			}
		}
		for x := 0; x < w; x++ {
			c := cells[x]
			switch {
			case !c.set:
				b.WriteString("\x1b[0m ")
			case c.patch:
				k := uint8(22 + 40*textPatch) // a little lighter for the themes that ask for it
				fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm%c", c.fg[0], c.fg[1], c.fg[2], k, k-2, k-4, c.ch)
			default:
				fmt.Fprintf(&b, "\x1b[0m\x1b[38;2;%d;%d;%dm%c", c.fg[0], c.fg[1], c.fg[2], c.ch)
			}
		}
		b.WriteString("\x1b[0m")
		if y < h-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// edgeColorClear is the picture's top-edge colour as it is, for the title bar.
func edgeColorClear(frame []byte, sw, sh int) [3]uint8 {
	if sw < 1 || sh < 1 || len(frame) < sw*3 {
		return [3]uint8{0, 0, 0}
	}
	rowsUsed := max(1, sh/12)
	var r, g, b, n float64
	for y := 0; y < rowsUsed; y++ {
		for x := 0; x < sw; x++ {
			i := (y*sw + x) * 3
			if i+2 >= len(frame) {
				break
			}
			r += float64(frame[i])
			g += float64(frame[i+1])
			b += float64(frame[i+2])
			n++
		}
	}
	if n == 0 {
		return [3]uint8{0, 0, 0}
	}
	return [3]uint8{uint8(r / n), uint8(g / n), uint8(b / n)}
}

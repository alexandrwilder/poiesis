package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Clear video: the camera frame is sent to the terminal as real pixels with the kitty
// graphics protocol, placed under the text across the whole window. Ghostty, kitty and
// WezTerm draw it; other terminals fall back to the styled, cell-drawn picture.

func terminalDrawsImages() bool {
	switch strings.ToLower(os.Getenv("TERM_PROGRAM")) {
	case "ghostty", "kitty", "wezterm":
		return true
	}
	return os.Getenv("KITTY_WINDOW_ID") != ""
}

const kittyChunk = 4096

// cropRect is the part of a w × h frame that has the window's shape: the picture is never
// stretched, the sides or the top and bottom are cut instead, evenly.
func cropRect(w, h, cols, rows int) (x, y, cw, ch int) {
	_, _, xpx, ypx := windowPixels()
	var aspect float64
	if xpx > 0 && ypx > 0 {
		aspect = float64(xpx) / float64(ypx)
	} else if cols > 0 && rows > 0 {
		aspect = float64(cols) / (float64(rows) * 2.1) // a typical cell is about twice as tall as wide
	} else {
		return 0, 0, w, h
	}
	src := float64(w) / float64(h)
	if aspect >= src { // the window is wider than the picture: cut top and bottom
		ch = int(float64(w) / aspect)
		if ch < 1 {
			ch = 1
		}
		return 0, (h - ch) / 2, w, ch
	}
	cw = int(float64(h) * aspect)
	if cw < 1 {
		cw = 1
	}
	return (w - cw) / 2, 0, cw, h
}

// kittyFrame hands the terminal one rgb24 frame to draw beneath the cells (z below the
// backgrounds, so a cell with a background colour covers it and an empty cell shows it), filling
// cols × rows cells, cropped to the window's shape. The frame goes through a temporary
// file the terminal reads and deletes itself, so the terminal's input carries only a note
// of a hundred bytes instead of a megabyte per frame. The same image id is reused, so each
// frame replaces the last. q=2 keeps the terminal from answering into the key stream.
func kittyFrame(rgb []byte, w, h, cols, rows int) string {
	x, y, cw, ch := cropRect(w, h, cols, rows)
	if os.Getenv("POIESIS_IMAGE_INLINE") != "1" {
		if s, ok := kittyFrameFile(rgb, w, h, cols, rows, x, y, cw, ch); ok {
			return s
		}
	}
	return kittyFrameInline(rgb, w, h, cols, rows, x, y, cw, ch)
}

var frameFileTurn int

// kittyFrameFile writes the frame where the terminal is allowed to read it: the temp
// folder, with the protocol's word in the name. Three names in turn, so a terminal that
// never reads them cannot fill the disk.
func kittyFrameFile(rgb []byte, w, h, cols, rows, x, y, cw, ch int) (string, bool) {
	frameFileTurn = (frameFileTurn + 1) % 3
	path := filepath.Join(os.TempDir(), fmt.Sprintf("tty-graphics-protocol-poiesis-%d-%d.rgb", os.Getpid(), frameFileTurn))
	if err := os.WriteFile(path, rgb, 0o600); err != nil {
		return "", false
	}
	enc := base64.StdEncoding.EncodeToString([]byte(path))
	return fmt.Sprintf("\x1b_Ga=T,t=t,f=24,s=%d,v=%d,x=%d,y=%d,w=%d,h=%d,i=1,q=2,c=%d,r=%d,z=-1073741825,C=1;%s\x1b\\",
		w, h, x, y, cw, ch, cols, rows, enc), true
}

// kittyFrameInline sends the pixels through the terminal's input, for terminals that
// cannot read files (or with POIESIS_IMAGE_INLINE=1).
func kittyFrameInline(rgb []byte, w, h, cols, rows, x, y, cw, ch int) string {
	var zb bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&zb, zlib.BestSpeed)
	zw.Write(rgb)
	zw.Close()
	data := base64.StdEncoding.EncodeToString(zb.Bytes())
	var b strings.Builder
	b.Grow(len(data) + len(data)/kittyChunk*32 + 96)
	first := true
	for len(data) > 0 {
		n := kittyChunk
		if n > len(data) {
			n = len(data)
		}
		chunk := data[:n]
		data = data[n:]
		more := 0
		if len(data) > 0 {
			more = 1
		}
		if first {
			fmt.Fprintf(&b, "\x1b_Ga=T,f=24,s=%d,v=%d,x=%d,y=%d,w=%d,h=%d,o=z,i=1,q=2,c=%d,r=%d,z=-1073741825,C=1,m=%d;%s\x1b\\", w, h, x, y, cw, ch, cols, rows, more, chunk)
			first = false
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d,q=2;%s\x1b\\", more, chunk)
		}
	}
	return b.String()
}

// kittyClear removes every image the log has placed.
const kittyClear = "\x1b_Ga=d,d=A,q=2\x1b\\"

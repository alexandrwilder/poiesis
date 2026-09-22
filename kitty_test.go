package main

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

// A frame must become well-formed image chunks: every chunk opens with ESC _ G and closes
// with ESC \, only the first carries the image keys, the last says m=0, and no chunk of
// data exceeds the protocol's 4096 bytes. A broken sequence could freeze the terminal.
func TestKittyFrameChunks(t *testing.T) {
	w, h := 64, 36
	rgb := make([]byte, w*h*3)
	for i := range rgb {
		rgb[i] = byte(i * 7) // not compressible to nothing
	}
	s := kittyFrameInline(rgb, w, h, 100, 30, 0, 0, w, h)
	chunks := strings.Split(s, "\x1b\\")
	chunks = chunks[:len(chunks)-1]
	if len(chunks) < 1 {
		t.Fatal("no chunks")
	}
	for i, c := range chunks {
		if !strings.HasPrefix(c, "\x1b_G") {
			t.Fatalf("chunk %d does not start with ESC _ G", i)
		}
		payload := c[strings.Index(c, ";")+1:]
		if len(payload) > 4096 {
			t.Errorf("chunk %d carries %d bytes, more than 4096", i, len(payload))
		}
		if i == 0 && !strings.Contains(c, "a=T,f=24,s=64,v=36,x=0,y=0,w=64,h=36,o=z,i=1,q=2,c=100,r=30,z=-1073741825,C=1") {
			t.Errorf("first chunk lacks the image keys: %.80s", c)
		}
		if i > 0 && strings.Contains(c, "a=T") {
			t.Errorf("chunk %d repeats the image keys", i)
		}
		wantMore := "m=1"
		if i == len(chunks)-1 {
			wantMore = "m=0"
		}
		if !strings.Contains(c[:strings.Index(c, ";")], wantMore) {
			t.Errorf("chunk %d of %d should say %s", i, len(chunks), wantMore)
		}
	}
	if !strings.HasPrefix(kittyClear, "\x1b_Ga=d,d=A") {
		t.Error("clear sequence is wrong")
	}
}

// The file route: a short note naming a temp file the terminal may read, and the file is
// really there with the frame in it. The name must carry the protocol's word.
func TestKittyFrameFile(t *testing.T) {
	w, h := 8, 4
	rgb := make([]byte, w*h*3)
	s, ok := kittyFrameFile(rgb, w, h, 100, 30, 0, 0, w, h)
	if !ok {
		t.Fatal("file route failed")
	}
	if !strings.HasPrefix(s, "\x1b_Ga=T,t=t,f=24,s=8,v=4,") || !strings.HasSuffix(s, "\x1b\\") {
		t.Fatalf("bad note: %q", s)
	}
	enc := s[strings.Index(s, ";")+1 : len(s)-2]
	path, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(path), "tty-graphics-protocol") {
		t.Errorf("the file name must contain the protocol's word: %s", path)
	}
	b, err := os.ReadFile(string(path))
	if err != nil || len(b) != w*h*3 {
		t.Errorf("frame file missing or wrong size: %v %d", err, len(b))
	}
	os.Remove(string(path))
}

// The crop keeps the window's shape: a wide window cuts top and bottom, a tall one the sides.
func TestCropRect(t *testing.T) {
	// no pixel sizes in tests: the cell guess applies (cols / (rows*2.1))
	x, y, cw, ch := cropRect(640, 360, 200, 20) // very wide
	if x != 0 || cw != 640 || ch >= 360 || y != (360-ch)/2 {
		t.Errorf("wide window: got x=%d y=%d w=%d h=%d", x, y, cw, ch)
	}
	x, y, cw, ch = cropRect(640, 360, 40, 40) // tall
	if y != 0 || ch != 360 || cw >= 640 || x != (640-cw)/2 {
		t.Errorf("tall window: got x=%d y=%d w=%d h=%d", x, y, cw, ch)
	}
}

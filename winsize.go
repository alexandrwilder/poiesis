//go:build !windows

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// windowPixels asks the terminal for its size in cells and in pixels, so the picture can
// be cropped to the window's true shape. Pixels are zero when the terminal does not say.
func windowPixels() (cols, rows, xpx, ypx int) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return 0, 0, 0, 0
	}
	return int(ws.Col), int(ws.Row), int(ws.Xpixel), int(ws.Ypixel)
}

package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// play opens the recording at a second. mpv seeks exactly; without it, the OS default
// player opens the file from the start and the status line says so.
func play(v *Vault, mediaRel string, seconds float64) (string, error) {
	path := v.Path(mediaRel)
	if mpv := findTool("mpv"); strings.Contains(mpv, "/") {
		cmd := exec.Command(mpv, "--really-quiet", "--force-window=yes", fmt.Sprintf("--start=%.1f", seconds), path)
		if err := cmd.Start(); err != nil {
			return "", err
		}
		go cmd.Wait()
		return fmt.Sprintf("playing from %s", mmss(seconds)), nil
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	go cmd.Wait()
	return fmt.Sprintf("opened in the default player (install mpv to jump to %s)", mmss(seconds)), nil
}

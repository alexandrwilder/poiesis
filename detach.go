//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach starts the child in its own session, so it outlives the terminal that asked for it
// and a Ctrl+C there never reaches it.
func detach(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

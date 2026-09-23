//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// detachedProcess is DETACHED_PROCESS: the child gets no console of its own.
const detachedProcess = 0x00000008

// detach starts the child in its own process group without a console, so it outlives the
// terminal that asked for it and a Ctrl+C there never reaches it.
func detach(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess}
}

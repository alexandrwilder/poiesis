//go:build !darwin

package main

// Bringing a window forward is a Mac thing for now; elsewhere a new window is opened.
func activateApp(pid int) bool { return false }

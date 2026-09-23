package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// `poiesis window` opens the app in its own clean window: no tab bar, no title bar, black,
// sized for the log. The window frame belongs to the terminal program, so this asks one of
// the terminals that can drop it (Ghostty, Alacritty, kitty, WezTerm; Omarchy ships the
// first two). Without any of them, macOS Terminal opens a black window that keeps its
// small title bar, and on other systems the app simply runs where it was started.

const (
	windowCols = 110
	windowRows = 34
	windowFont = 14
)

// windowPIDFile holds the process id of the running log window, written by the app itself.
func windowPIDFile() string { return filepath.Join(appStateDir(), "window.pid") }

// commandFile is how the menu bar item talks to a window that is already open.
func commandFile() string { return filepath.Join(appStateDir(), "command") }

// focusWindow brings an open log window to the front. It returns false when none is open.
func focusWindow() bool {
	dbg := os.Getenv("POIESIS_DEBUG") == "1"
	b, err := os.ReadFile(windowPIDFile())
	if err != nil {
		if dbg {
			fmt.Fprintln(os.Stderr, "focus: no window.pid:", err)
		}
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || !processAlive(pid) {
		if dbg {
			fmt.Fprintf(os.Stderr, "focus: pid %q not alive (%v)\n", strings.TrimSpace(string(b)), err)
		}
		return false
	}
	if dbg {
		chain := []int{pid}
		for p, i := parentPID(pid), 0; p > 1 && i < 6; i, p = i+1, parentPID(p) {
			chain = append(chain, p)
		}
		fmt.Fprintln(os.Stderr, "focus: process chain", chain)
	}
	// the window belongs to the terminal program: walk up to it and bring that to the front
	for p, i := pid, 0; p > 1 && i < 6; i++ {
		if activateApp(p) {
			return true
		}
		p = parentPID(p)
	}
	return false
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func parentPID(pid int) int {
	out, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
}

// openOrFocus is what the menu bar item does: one window, brought forward if it is open.
// `what` is "" to just show it, or "record" to put it on the record screen.
func openOrFocus(root, what string) error {
	if app := macHostApp(); app != "" {
		return openHostApp(app, what)
	}
	if focusWindow() {
		if what != "" {
			_ = os.MkdirAll(appStateDir(), 0o755)
			_ = os.WriteFile(commandFile(), []byte(what), 0o644)
		}
		return nil
	}
	return openWindow(root, what == "record")
}

// macHostApp is Poiesis.app when its main program is the Mac host (hosts/mac), else "".
func macHostApp() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	if app := outerAppBundle(); app != "" && fileThere(filepath.Join(app, "Contents", "MacOS", "PoiesisHost")) {
		return app
	}
	return ""
}

// openHostApp opens the Mac host's window or brings it forward: it is a regular app, so
// macOS does both reliably. A core already running in it reads the command file.
func openHostApp(app, what string) error {
	if what != "" {
		_ = os.MkdirAll(appStateDir(), 0o755)
		_ = os.WriteFile(commandFile(), []byte(what), 0o644)
	}
	return exec.Command("open", "-a", app).Run()
}

func openWindow(root string, record bool) error {
	if app := macHostApp(); app != "" {
		what := ""
		if record {
			what = "record"
		}
		return openHostApp(app, what)
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	// prefer the app's own binary: its path has no space, so the terminal gets the program
	// directly and no shell is involved
	if runtime.GOOS == "darwin" {
		if p := macAppBinary(); p != "" && p != self {
			self = p
		}
	}
	app := []string{self, "ui"}
	if record {
		app = append(app, "--record")
	}
	// inside Poiesis.app the vault path comes from the bundle, so the command carries no
	// spaces and no shell is needed: fewer moving parts, and macOS never sees a script
	if !(inMacApp() && bundleVault() == root) {
		app = append(app, "--vault", root)
	}
	if bin := findGhostty(); bin != "" {
		// Ghostty asks before running a command another app hands it on the command line,
		// so the command goes in a settings file of our own instead: same window, no dialog.
		cfg, err := writeGhosttyConfig(app)
		if err != nil {
			return err
		}
		if runtime.GOOS == "darwin" {
			bin = filepath.Join(bin, "Contents", "MacOS", "ghostty")
		}
		return startDetached(bin, "--config-file="+cfg)
	}
	if bin, err := exec.LookPath("alacritty"); err == nil {
		args := []string{"-o", "window.decorations=\"None\"", "-o", fmt.Sprintf("window.dimensions.columns=%d", windowCols),
			"-o", fmt.Sprintf("window.dimensions.lines=%d", windowRows), "-o", fmt.Sprintf("font.size=%d", windowFont),
			"-o", "colors.primary.background=\"#000000\"", "-T", "Poiesis", "-e"}
		return exec.Command(bin, append(args, app...)...).Start()
	}
	if bin, err := exec.LookPath("kitty"); err == nil {
		args := []string{"-o", "hide_window_decorations=yes", "-o", fmt.Sprintf("initial_window_width=%dc", windowCols),
			"-o", fmt.Sprintf("initial_window_height=%dc", windowRows), "-o", fmt.Sprintf("font_size=%d", windowFont),
			"-o", "background=#000000", "-T", "Poiesis"}
		return exec.Command(bin, append(args, app...)...).Start()
	}
	if bin, err := exec.LookPath("wezterm"); err == nil {
		args := []string{"--config", "window_decorations=\"NONE\"", "--config", "enable_tab_bar=false",
			"--config", fmt.Sprintf("initial_cols=%d", windowCols), "--config", fmt.Sprintf("initial_rows=%d", windowRows),
			"--config", fmt.Sprintf("font_size=%d", windowFont), "--config", "colors={background=\"#000000\"}", "start", "--"}
		return exec.Command(bin, append(args, app...)...).Start()
	}
	if runtime.GOOS == "darwin" {
		return openMacTerminal(app)
	}
	return fmt.Errorf("no terminal found that can open a clean window (Ghostty, Alacritty, kitty or WezTerm); run `poiesis` in your terminal instead")
}

// findGhostty returns the Ghostty app (macOS) or binary (elsewhere), or "".
func findGhostty() string {
	if runtime.GOOS == "darwin" {
		// the copy inside Poiesis.app first: it carries this app's name
		if t := embeddedTerminal(); fileThere(t) {
			return t
		}
		for _, p := range []string{"/Applications/Ghostty.app", filepath.Join(os.Getenv("HOME"), "Applications", "Ghostty.app")} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		return ""
	}
	if bin, err := exec.LookPath("ghostty"); err == nil {
		return bin
	}
	return ""
}

// openMacTerminal opens Terminal.app with a black window sized for the log. Terminal keeps
// its title bar; everything else is quiet. The window closes when the app quits.
func openMacTerminal(app []string) error {
	var quoted []string
	for _, a := range app {
		quoted = append(quoted, "'"+strings.ReplaceAll(a, "'", "'\\''")+"'")
	}
	cmd := "clear; " + strings.Join(quoted, " ") + "; exit"
	script := fmt.Sprintf(`tell application "Terminal"
  activate
  set w to do script "%s"
  set background color of w to {0, 0, 0}
  set normal text color of w to {59000, 58500, 56500}
  set font name of w to "Menlo"
  set font size of w to %d
  set number of columns of w to %d
  set number of rows of w to %d
  set title displays custom title of w to true
  set custom title of w to "Poiesis"
  set title displays device name of w to false
  set title displays shell path of w to false
  set title displays window size of w to false
end tell`, strings.ReplaceAll(cmd, `"`, `\"`), windowFont, windowCols, windowRows)
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Terminal: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// shellJoin quotes each argument for /bin/sh.
func shellJoin(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = "'" + strings.ReplaceAll(a, "'", "'\\''") + "'"
	}
	return strings.Join(q, " ")
}

// macAppBinary is the program inside Poiesis.app, or "" when it is not installed.
func macAppBinary() string {
	app := outerAppBundle()
	if app == "" {
		return ""
	}
	p := filepath.Join(app, "Contents", "MacOS", "poiesis")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// writeGhosttyConfig puts the command and the Poiesis look in the app's own folder.
func writeGhosttyConfig(app []string) (string, error) {
	dir := appStateDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	cfg := filepath.Join(dir, "window.conf")
	icon := ""
	if app := outerAppBundle(); app != "" {
		if p := filepath.Join(app, "Contents", "Resources", "icon.png"); fileThere(p) {
			icon = "macos-icon = custom\nmacos-custom-icon = " + p + "\n"
		}
	}
	body := "command = " + shellJoin(app) + "\n" + icon + `window-decoration = auto
# the title bar keeps the three buttons and the drag area; Poiesis tints it with the picture's top edge
macos-titlebar-style = transparent
macos-titlebar-proxy-icon = hidden
background = 000000
foreground = e6e4dc
title = Poiesis
confirm-close-surface = false
quit-after-last-window-closed = true
# Poiesis updates as one piece: the window host must never ask about updates
auto-update = off
# no margin around the cells: the picture reaches the window's edges
window-padding-x = 0
window-padding-y = 0
window-padding-balance = false
# the sliver left over when the window is not a whole number of cells takes the edge colour
window-padding-color = extend
# one window, one log: no tabs, no splits, no second window
keybind = super+t=unbind
keybind = super+n=unbind
keybind = super+shift+n=unbind
keybind = super+d=unbind
keybind = super+shift+d=unbind
keybind = super+shift+t=unbind
` + fmt.Sprintf("font-size = %d\nwindow-width = %d\nwindow-height = %d\n", windowFont, windowCols, windowRows)
	return cfg, os.WriteFile(cfg, []byte(body), 0o644)
}

// startDetached starts the terminal in its own session so it outlives whoever asked for it.
func startDetached(bin string, args ...string) error {
	c := exec.Command(bin, args...)
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return c.Start()
}

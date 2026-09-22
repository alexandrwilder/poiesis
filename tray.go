package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// `log_ tray` puts LOG_ in the menu bar (Mac), the tray (Windows) or the bar's tray
// (Omarchy, waybar): the streak at a glance, Open, Record now, Quit. `--login` makes it
// start when you log in. It reads only the entry names in the vault, nothing else.

//go:embed assets/tray-template.png
var trayTemplate []byte

//go:embed assets/tray.png
var trayColour []byte

//go:embed assets/icon.ico
var trayICO []byte

// what the menu does, shared by every system
func trayOpen(root string)   { _ = openOrFocus(root, "") }
func trayRecord(root string) { _ = openOrFocus(root, "record") }

// runTray draws the item. On a Mac it must run from inside an app bundle (LOG_.app carries
// a helper for it); a crowded menu bar may hide it behind the notch on a laptop screen.
func runTray(root string, login bool) error {
	if login {
		p, err := installLoginItem(root)
		if err != nil {
			return err
		}
		fmt.Println("LOG_ tray starts at login and is running now:", p)
		return nil // launchd runs it from here on
	}
	return trayRunNative(root)
}

// trayStreak reads the line the app leaves in its own folder. The menu bar item never
// touches the vault, so macOS never asks it for your Documents folder.
func trayStreak(root string) string {
	b, err := os.ReadFile(filepath.Join(appStateDir(), "streak.txt"))
	if err != nil || len(b) == 0 {
		return "no days logged yet"
	}
	return strings.TrimSpace(string(b))
}

// installLoginItem makes the tray start at login, the way each system expects.
func installLoginItem(root string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		// only a program inside an app bundle gets a menu bar item
		bundled := macMenuBinary()
		if _, err := os.Stat(bundled); err != nil {
			return "", fmt.Errorf("run `log_ setup --app` first: the menu bar item needs LOG_.app")
		}
		self = bundled
	}
	switch runtime.GOOS {
	case "darwin":
		dir := filepath.Join(home, "Library", "LaunchAgents")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		p := filepath.Join(dir, "app.logunderscore.tray.plist")
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>app.logunderscore.tray</string>
  <key>AssociatedBundleIdentifiers</key><array><string>app.logunderscore</string><string>app.logunderscore.menu</string></array>
  <key>ProgramArguments</key><array><string>%s</string><string>tray</string><string>--vault</string><string>%s</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><false/>
</dict></plist>
`, self, root)
		if err := os.WriteFile(p, []byte(plist), 0o644); err != nil {
			return "", err
		}
		_ = exec.Command("launchctl", "bootout", "gui/"+uid(), p).Run()
		if out, err := exec.Command("launchctl", "bootstrap", "gui/"+uid(), p).CombinedOutput(); err != nil {
			return "", fmt.Errorf("launchctl: %s", strings.TrimSpace(string(out)))
		}
		return p, nil
	case "windows":
		dir := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
		p := filepath.Join(dir, "LOG_ tray.cmd")
		cmd := fmt.Sprintf("@start \"\" \"%s\" tray --vault \"%s\"\r\n", self, root)
		return p, os.WriteFile(p, []byte(cmd), 0o644)
	default:
		dir := filepath.Join(home, ".config", "autostart")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		p := filepath.Join(dir, "log_-tray.desktop")
		desktop := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=LOG_ tray\nExec=%s tray --vault %q\nX-GNOME-Autostart-enabled=true\n", self, root)
		return p, os.WriteFile(p, []byte(desktop), 0o644)
	}
}

func uid() string { return fmt.Sprint(os.Getuid()) }

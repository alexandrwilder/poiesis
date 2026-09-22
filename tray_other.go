//go:build !darwin

package main

import (
	"runtime"
	"time"

	"fyne.io/systray"
)

// On Windows and Linux the tray library works: it talks to the tray protocol directly.
func trayRunNative(root string) error {
	systray.Run(func() {
		if runtime.GOOS == "windows" {
			systray.SetIcon(trayICO)
		} else {
			systray.SetIcon(trayColour)
		}
		systray.SetTitle("Poiesis")
		systray.SetTooltip("Poiesis")
		streak := systray.AddMenuItem(trayStreak(root), "")
		streak.Disable()
		systray.AddSeparator()
		open := systray.AddMenuItem("Open Poiesis", "the log in its own window")
		record := systray.AddMenuItem("Record now", "open straight on the record screen")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "")
		refresh := time.NewTicker(10 * time.Minute)
		go func() {
			for {
				select {
				case <-open.ClickedCh:
					trayOpen(root)
				case <-record.ClickedCh:
					trayRecord(root)
				case <-refresh.C:
					streak.SetTitle(trayStreak(root))
				case <-quit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, func() {})
	return nil
}

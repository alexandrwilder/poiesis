package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// streakText: "12 days in a row · 47 days logged"; withToday counts an entry made now.
func (m *tuiModel) streakText(withToday bool) string {
	var at []string
	for _, e := range m.data.entries {
		at = append(at, e.RecordedAt)
	}
	now := time.Now()
	if withToday {
		at = append(at, now.Format(time.RFC3339))
	}
	s := daysLogged(at, now)
	if s.Days == 0 {
		return "no days logged yet"
	}
	run := fmt.Sprintf("%d days in a row", s.Run)
	if s.Run == 1 {
		run = "day 1 of a new run"
	}
	days := fmt.Sprintf("%d days logged", s.Days)
	if s.Days == 1 {
		days = "1 day logged"
	}
	return fmt.Sprintf("%s  ·  %s", run, days)
}

// cacheStreak keeps the streak line in the app's own folder, so the menu bar item can show
// it without ever reading the vault.
func (m *tuiModel) cacheStreak() {
	_ = os.MkdirAll(appStateDir(), 0o755)
	_ = os.WriteFile(filepath.Join(appStateDir(), "streak.txt"), []byte(m.streakText(false)), 0o644)
}

// traceKey appends every key to ~/Library/Logs/LOG_keys.log when LOG_DEBUG_KEYS=1, to see
// what a terminal sends the app on its own.
func traceKey(k tea.KeyMsg) {
	if os.Getenv("LOG_DEBUG_KEYS") != "1" {
		return
	}
	f, err := os.OpenFile(filepath.Join(os.Getenv("HOME"), "Library", "Logs", "LOG_keys.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %q type=%d\n", time.Now().Format("15:04:05.000"), k.String(), k.Type)
}

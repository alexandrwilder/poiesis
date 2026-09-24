package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func writeJSON(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The door: settings lists the AI apps that can read the log, and closing one takes Poiesis
// out of that app's own settings, keeps everything else in them, and keeps a copy as it was.
func TestTheDoorListsAndClosesAIApps(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cursor := filepath.Join(home, ".cursor", "mcp.json")
	writeJSON(t, cursor, `{"mcpServers":{"poiesis":{"command":"/Users/x/.local/bin/poiesis","args":["mcp"]},"notes":{"command":"notes-server"}}}`)
	writeJSON(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"poiesis":{"command":"/Users/x/.local/bin/poiesis","args":["mcp"]}},"theme":"dark"}`)
	removed := 0
	saved := claudeCodeRemove
	claudeCodeRemove = func() error { removed++; return nil }
	defer func() { claudeCodeRemove = saved }()

	names := aiAppNames(connectedAIApps())
	if !strings.Contains(names, "Claude Code") || !strings.Contains(names, "Cursor") {
		t.Fatalf("the door list: %q", names)
	}
	for _, a := range connectedAIApps() {
		if err := closeDoor(a); err != nil {
			t.Fatal(err)
		}
	}
	if removed != 1 {
		t.Fatalf("Claude Code was asked to forget Poiesis %d times", removed)
	}
	b, _ := os.ReadFile(cursor)
	if strings.Contains(string(b), "poiesis") || !strings.Contains(string(b), "notes-server") {
		t.Fatalf("Cursor's settings after closing: %s", b)
	}
	if before, _ := os.ReadFile(cursor + ".before-poiesis-removed"); !strings.Contains(string(before), "poiesis") {
		t.Fatal("no copy of Cursor's settings as they were")
	}
	for _, a := range connectedAIApps() {
		if a.name == "Cursor" {
			t.Fatal("Cursor still listed after its door was closed")
		}
	}
	if runtime.GOOS == "darwin" && len(knownAIApps()) != 3 {
		t.Fatalf("on a Mac, Claude Desktop is looked for too: %v", knownAIApps())
	}
}

// In settings the door closes on the second enter only, and any other key keeps it open.
func TestSettingsClosesADoorOnTheSecondEnter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cursor := filepath.Join(home, ".cursor", "mcp.json")
	writeJSON(t, cursor, `{"mcpServers":{"poiesis":{"command":"poiesis","args":["mcp"]}}}`)
	m := &tuiModel{v: &Vault{Root: t.TempDir(), Config: defaultConfig()}, data: &tuiData{}}
	m.settings.loaded = true // no device listing in a test
	m.enterSettings()
	if len(m.settings.apps) != 1 {
		t.Fatalf("apps: %v", m.settings.apps)
	}
	m.settings.cursor = rowCount // Cursor's row
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	m.updateSettings(enter)
	m.updateSettings(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}) // changed their mind
	m.updateSettings(enter)
	if b, _ := os.ReadFile(cursor); !strings.Contains(string(b), "poiesis") {
		t.Fatal("the door closed without a second enter in a row")
	}
	m.updateSettings(enter)
	if b, _ := os.ReadFile(cursor); strings.Contains(string(b), "poiesis") {
		t.Fatalf("the second enter did not close the door: %s", b)
	}
}

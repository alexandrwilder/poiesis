package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The daily look happens only when the setting allows it, and at most once in most of a day.
func TestUpdateCheckFollowsTheSetting(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the stamp lives in the app's own folder
	c := defaultConfig()
	if !updateCheckDue(c) {
		t.Fatal("a copy that never looked should look, by default")
	}
	c.UpdateCheck = "off"
	if updateCheckDue(c) {
		t.Fatal("with updates off it must never look")
	}
	c.UpdateCheck = "daily"
	stamp := filepath.Join(appStateDir(), "update-checked")
	if err := os.MkdirAll(filepath.Dir(stamp), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stamp, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if updateCheckDue(c) {
		t.Fatal("it looked a moment ago; it must not look again")
	}
	old := time.Now().Add(-21 * time.Hour)
	if err := os.Chtimes(stamp, old, old); err != nil {
		t.Fatal(err)
	}
	if !updateCheckDue(c) {
		t.Fatal("a day later it should look again")
	}
}

// A newer version shows in the frame; enter installs it, but never during an entry.
func TestANewerVersionWaitsForTheEntry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := &tuiModel{v: &Vault{Config: defaultConfig()}, data: &tuiData{}}
	if strings.Contains(m.frameNote(), "ready") {
		t.Fatal("nothing found, nothing to say")
	}
	if _, ok := m.updateMsg(updateFoundMsg{"0.0.9"}); !ok || !strings.Contains(m.frameNote(), "0.0.9 is ready") {
		t.Fatalf("the frame does not tell about 0.0.9: %q", m.frameNote())
	}
	m.record.processing = true
	if cmd := m.installUpdate(); cmd != nil || !strings.Contains(m.status, "waits") {
		t.Fatalf("an update started while an entry was processed (status %q)", m.status)
	}
	m.record.processing, m.record.phase = false, "recording"
	if cmd := m.installUpdate(); cmd != nil {
		t.Fatal("an update started while recording")
	}
}

package main

import "testing"

// The limit comes from the vault settings, with three minutes as the default.
func TestMaxEntry(t *testing.T) {
	m := &tuiModel{v: &Vault{Config: defaultConfig()}}
	if got := m.maxEntry().Seconds(); got != 180 {
		t.Errorf("default limit is %.0fs, want 180s", got)
	}
	m.v.Config.MaxEntryS = 300
	if got := m.maxEntry().Seconds(); got != 300 {
		t.Errorf("configured limit is %.0fs, want 300s", got)
	}
	m.v.Config.MaxEntryS = 0
	if got := m.maxEntry().Seconds(); got != 180 {
		t.Errorf("zero should fall back to 180s, got %.0fs", got)
	}
}

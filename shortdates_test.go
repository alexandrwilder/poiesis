package main

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// A log a person edited by hand, or a sync left half-written, may hold a date cut short. The
// pages, the index and the AI connection still work; the date shows as it is.
func TestADateCutShortNeverCrashesAPage(t *testing.T) {
	v, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	page := "---\nid: 2026-09-24-a\nday: 1\nrecorded_at: \"2026-09\"\nduration_s: 60\n---\n"
	if err := os.WriteFile(v.Path("episodes", "2026-09-24-a.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	claim := `{"id":"c1","kind":"event","text":"the oven","quote":"the oven","about":["bakery"],"stated_at":"2026","source":{"episode":"2026-09-24-a","start":1,"end":2}}` + "\n"
	if err := os.WriteFile(v.Path("episodes", "2026-09-24-a.claims.jsonl"), []byte(claim), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v.Path("entities", "bakery.md"), []byte("---\nid: bakery\nkind: place\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := v.loadEntities(); err != nil {
		t.Fatal(err)
	}
	if err := rebuildEntityPages(v); err != nil {
		t.Fatal(err)
	}
	if err := writeIndex(v); err != nil {
		t.Fatal(err)
	}
	if _, err := v.mcpSearch(searchIn{Query: "oven"}); err != nil {
		t.Fatal(err)
	}
	if _, err := v.mcpMoment(momentIn{Episode: "2026-09-24-a", T: 1}); err != nil {
		t.Logf("moment: %v (an error is fine, a crash is not)", err)
	}
}

// e on an entry whose words could not be read does nothing, instead of stopping the app.
func TestTheEKeyOnAnEntryWithoutWords(t *testing.T) {
	m := &tuiModel{v: &Vault{Config: defaultConfig()}, data: &tuiData{}}
	m.scr = screenEntry
	m.entry = entryState{loadErr: "transcript not readable"}
	m.updateEntry(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
}

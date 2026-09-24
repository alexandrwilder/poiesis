package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// a log with one entry and two claims: one right, one the person will say is wrong
func logWithTwoClaims(t *testing.T) *Vault {
	t.Helper()
	v, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	page := "---\nid: 2026-09-24-a\nday: 1\nrecorded_at: \"2026-09-24T20:00:00+02:00\"\nduration_s: 60\n---\n"
	if err := os.WriteFile(v.Path("episodes", "2026-09-24-a.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	claims := `{"id":"right","kind":"event","text":"The whole family loved the stew.","quote":"alla älskade grytan","about":["family"],"stated_at":"2026-09-24T20:00:00+02:00","source":{"episode":"2026-09-24-a","start":3,"end":5}}
{"id":"wrong","kind":"decision","text":"I decided to sell the bakery.","quote":"jag tror kanske att sälja","about":["bakery"],"stated_at":"2026-09-24T20:00:00+02:00","source":{"episode":"2026-09-24-a","start":9,"end":12}}
`
	if err := os.WriteFile(v.Path("episodes", "2026-09-24-a.claims.jsonl"), []byte(claims), 0o644); err != nil {
		t.Fatal(err)
	}
	return v
}

// A claim the person marks wrong never comes back: not in the log, not in Earlier, not to an
// AI. The mark can be taken back, and the claims file itself is never edited.
func TestAWrongClaimNeverComesBack(t *testing.T) {
	v := logWithTwoClaims(t)
	before, _ := os.ReadFile(v.Path("episodes", "2026-09-24-a.claims.jsonl"))
	if err := markClaim(v, "2026-09-24-a", "wrong", true, time.Now()); err != nil {
		t.Fatal(err)
	}
	all, err := AllClaims(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != "right" {
		t.Fatalf("after the mark: %+v", all)
	}
	if s, err := v.mcpSearch(searchIn{Query: "bakery"}); err != nil || strings.Contains(s, "sell the bakery") {
		t.Fatalf("an AI still finds the wrong claim: %v\n%s", err, s)
	}
	if err := markClaim(v, "2026-09-24-a", "wrong", false, time.Now()); err != nil {
		t.Fatal(err)
	}
	if all, _ := AllClaims(v); len(all) != 2 {
		t.Fatalf("taking the mark back did not bring the claim back: %d claims", len(all))
	}
	if after, _ := os.ReadFile(v.Path("episodes", "2026-09-24-a.claims.jsonl")); string(after) != string(before) {
		t.Fatal("the claims file was edited")
	}
}

// The read-back lists what the log heard, in the order it was said; w marks the chosen line
// wrong, and w again takes the mark back.
func TestTheReadBackMarksALineWrong(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	v := logWithTwoClaims(t)
	d, err := loadTUIData(v)
	if err != nil {
		t.Fatal(err)
	}
	m := &tuiModel{v: v, data: d}
	m.openEntry(0)
	if len(m.entry.heard) != 2 || m.entry.heard[1].ID != "wrong" {
		t.Fatalf("heard: %+v", m.entry.heard)
	}
	m.updateEntry(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m.updateEntry(tea.KeyMsg{Type: tea.KeyDown})
	m.updateEntry(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if !m.entry.wrong["wrong"] {
		t.Fatalf("w did not mark the line: %v", m.entry.wrong)
	}
	if marks, _ := wrongClaims(v, "2026-09-24-a"); !marks["wrong"] || marks["right"] {
		t.Fatalf("the mark was not kept: %v", marks)
	}
	m.updateEntry(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if marks, _ := wrongClaims(v, "2026-09-24-a"); marks["wrong"] {
		t.Fatal("w again did not take the mark back")
	}
}

// An entry that was just processed opens on what the log heard; opening it any other way
// shows the words.
func TestANewEntryOpensOnWhatWasHeard(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	v := logWithTwoClaims(t)
	d, err := loadTUIData(v)
	if err != nil {
		t.Fatal(err)
	}
	m := &tuiModel{v: v, data: &tuiData{}}
	m.Update(dataReloadedMsg{data: d, openEntry: "2026-09-24-a", readback: true})
	if m.scr != screenEntry || !m.entry.readback {
		t.Fatalf("screen %v, read-back %v", m.scr, m.entry.readback)
	}
	m.Update(dataReloadedMsg{data: d, openEntry: "2026-09-24-a"})
	if m.entry.readback {
		t.Fatal("a reload that was not a new entry opened the read-back")
	}
}

// The entry's own page, which an AI can read whole, loses the claim the person marked wrong;
// everything else on it stays, a note added by hand and an unknown header line included.
func TestAWrongClaimLeavesTheEntrysPage(t *testing.T) {
	v := logWithTwoClaims(t)
	heard, _, err := heardIn(v, "2026-09-24-a")
	if err != nil {
		t.Fatal(err)
	}
	ep := &Episode{ID: "2026-09-24-a", Day: 1, RecordedAt: "2026-09-24T20:00:00+02:00", DurationS: 60, ClaimCount: 2}
	lines := []Line{{Start: 3, Text: "alla älskade grytan"}, {Start: 9, Text: "jag tror kanske att sälja"}}
	if err := writeEpisode(v, ep, lines, heard); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(v.Path("episodes", "2026-09-24-a.md"))
	page := strings.Replace(string(b), "day: 1\n", "day: 1\nmood: calm\n", 1) + "\n## My note\n\nCall grandmother about the recipe.\n"
	if err := os.WriteFile(v.Path("episodes", "2026-09-24-a.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := markClaim(v, "2026-09-24-a", "wrong", true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if msg := rebuildPagesCmd(v.Root, "2026-09-24-a")(); msg.(pagesRebuiltMsg).err != nil {
		t.Fatal(msg.(pagesRebuiltMsg).err)
	}
	b, _ = os.ReadFile(v.Path("episodes", "2026-09-24-a.md"))
	got := string(b)
	for _, want := range []string{"loved the stew", "jag tror kanske att sälja", "mood: calm", "Call grandmother about the recipe.", "claims: 1"} {
		if !strings.Contains(got, want) {
			t.Errorf("the page lost %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "sell the bakery") {
		t.Errorf("the page still holds the wrong claim:\n%s", got)
	}
	if s, _ := v.mcpRead("2026-09-24-a"); strings.Contains(s, "sell the bakery") {
		t.Fatal("an AI reading the page still meets the wrong claim")
	}
}

// An entry with nothing sorted out of it: the read-back says so, and no key stops the app.
func TestTheReadBackOfAnEntryWithoutClaims(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	v, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	page := "---\nid: 2026-09-24-b\nday: 1\nrecorded_at: \"2026-09-24T21:00:00+02:00\"\nduration_s: 30\n---\n"
	if err := os.WriteFile(v.Path("episodes", "2026-09-24-b.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := loadTUIData(v)
	if err != nil {
		t.Fatal(err)
	}
	m := &tuiModel{v: v, data: d}
	m.openEntry(0)
	m.updateEntry(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	for _, k := range []tea.KeyMsg{{Type: tea.KeyEnd}, {Type: tea.KeyPgDown}, {Type: tea.KeyEnter}, {Type: tea.KeyRunes, Runes: []rune("w")}} {
		m.updateEntry(k)
		if m.entry.heardC.cursor != 0 {
			t.Fatalf("after %s the cursor left the list: %d", k.String(), m.entry.heardC.cursor)
		}
	}
}

// An entry moved to the trash takes the person's marks with it.
func TestTheMarksGoToTheTrashWithTheEntry(t *testing.T) {
	v := logWithTwoClaims(t)
	if err := markClaim(v, "2026-09-24-a", "wrong", true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := TrashEntry(v, Episode{ID: "2026-09-24-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(correctionsPath(v, "2026-09-24-a")); !os.IsNotExist(err) {
		t.Fatal("the marks stayed behind in episodes/")
	}
	found, _ := filepath.Glob(v.Path("trash", "*_2026-09-24-a", "2026-09-24-a.corrections.jsonl"))
	if len(found) != 1 {
		t.Fatalf("the marks are not in the trash: %v", found)
	}
}

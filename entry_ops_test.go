package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Handling an entry must be honest: changing the mission rewrites only that line, and
// trashing moves the page, the video and that entry's claims out of the vault, leaving
// every other claim untouched.
func TestSetMissionAndTrash(t *testing.T) {
	dir := t.TempDir()
	v, err := OpenVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	ep := Episode{ID: "2026-09-03-a", Day: 1, RecordedAt: "2026-09-03T08:00:00+02:00",
		Media: "raw/2026/09/clip.mp4", Mission: "bageriet", ClaimCount: 1}
	fm, err := frontmatter(ep)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v.Path("episodes", ep.ID+".md"), []byte(fm+"\n# entry\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(v.Path("raw", "2026", "09"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v.Path(ep.Media), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	mine := `{"id":"a","kind":"event","text":"mine","source":{"episode":"2026-09-03-a","start":1}}` + "\n"
	other := `{"id":"b","kind":"event","text":"someone else's","source":{"episode":"2026-09-02-a","start":2}}` + "\n"
	if err := os.WriteFile(v.Path("episodes", ep.ID+".claims.jsonl"), []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v.Path("episodes", "2026-09-02-a.claims.jsonl"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SetMission(v, ep.ID, "sourdough"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(v.Path("episodes", ep.ID+".md"))
	if !strings.Contains(string(b), "mission: sourdough") || strings.Contains(string(b), "mission: bageriet") {
		t.Errorf("mission not rewritten: %s", firstLines(string(b), 12))
	}

	if err := TrashEntry(v, ep); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(v.Path("episodes", ep.ID+".md")); err == nil {
		t.Error("the entry page is still in the vault")
	}
	if _, err := os.Stat(v.Path(ep.Media)); err == nil {
		t.Error("the video is still in the vault")
	}
	if _, err := os.Stat(v.Path("episodes", ep.ID+".claims.jsonl")); err == nil {
		t.Error("the trashed entry's claims file is still in the vault")
	}
	if left, err := os.ReadFile(v.Path("episodes", "2026-09-02-a.claims.jsonl")); err != nil || !strings.Contains(string(left), `"id":"b"`) {
		t.Error("another entry's claims were touched")
	}
	all, err := AllClaims(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range all {
		if c.Source.Episode == ep.ID {
			t.Error("the app still sees the trashed entry's claims")
		}
	}
	trashed, _ := filepath.Glob(v.Path("trash", "*", "*"))
	var names []string
	for _, p := range trashed {
		names = append(names, filepath.Base(p))
	}
	for _, want := range []string{ep.ID + ".md", "clip.mp4", ep.ID + ".claims.jsonl"} {
		if !contains(names, want) {
			t.Errorf("%s is not in the trash folder (found %v)", want, names)
		}
	}
}

func firstLines(s string, n int) string {
	parts := strings.SplitN(s, "\n", n+1)
	if len(parts) > n {
		parts = parts[:n]
	}
	return strings.Join(parts, "\n")
}

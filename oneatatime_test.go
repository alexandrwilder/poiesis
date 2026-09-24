package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Entries recorded close together are processed one after the other, and the record screen
// says processing until the last one is done.
func TestEntriesAreProcessedOneAtATime(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	countCameraOpens(t)
	m := &tuiModel{v: &Vault{Config: defaultConfig()}, data: &tuiData{}}
	m.record.phase, m.record.processing, m.record.pending = "ready", true, 2
	m.Update(ingestDoneMsg{err: errors.New("the first one failed")})
	if !m.record.processing || m.record.pending != 1 {
		t.Fatalf("after the first: processing %v, pending %d", m.record.processing, m.record.pending)
	}
	m.Update(ingestDoneMsg{err: errors.New("the second one failed")})
	if m.record.processing || m.record.pending != 0 {
		t.Fatalf("after the second: processing %v, pending %d", m.record.processing, m.record.pending)
	}
}

// a log with one person in it
func logWithErik(t *testing.T) *Vault {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "entities"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "entities", "erik.md"), []byte("---\nid: erik\nkind: person\n---\n# Erik\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := OpenVault(root)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// An AI app may ask several things at once. Run with -race: search reloads the people and
// things while read walks them, and the connection must answer one call at a time.
func TestTheAIConnectionAnswersParallelCalls(t *testing.T) {
	v := logWithErik(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = v.mcpSearch(searchIn{Query: "erik"}) }()
		go func() { defer wg.Done(); _, _ = v.mcpRead("erik") }()
	}
	wg.Wait()
}

// A reload after an entry reads into a vault of its own. Run with -race: the window's map is
// only swapped on the window's goroutine, never written by the reload.
func TestAReloadLeavesTheWindowsMapAlone(t *testing.T) {
	v := logWithErik(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		msg := reloadCmd(v, "", "")()
		if r, ok := msg.(dataReloadedMsg); !ok || r.entities["erik"] == nil {
			t.Errorf("the reload did not read the log: %+v", msg)
		}
	}()
	for i := 0; i < 200; i++ {
		v.Entities["erik"] = &Entity{ID: "erik"} // the window's own use of its map
	}
	<-done
}

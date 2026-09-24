package main

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// An update that finishes while an entry is underway never cuts it short: the app starts
// again once the entry is done.
func TestAnUpdateWaitsForTheEntry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	countCameraOpens(t)
	var reopened []string
	saved := reopenApp
	reopenApp = func(app string) { reopened = append(reopened, app) }
	defer func() { reopenApp = saved }()

	m := &tuiModel{v: &Vault{Config: defaultConfig()}, data: &tuiData{}}
	m.record.phase = "recording"
	if _, cmd := m.Update(updateInstalledMsg{version: "0.2.0", reopen: "/Applications/Poiesis.app"}); cmd != nil {
		if _, quits := cmd().(tea.QuitMsg); quits {
			t.Fatal("the update quit in the middle of a recording")
		}
	}
	if len(reopened) != 0 || m.reopenAfter == "" {
		t.Fatalf("reopened %v, waiting for %q", reopened, m.reopenAfter)
	}
	m.record.phase, m.record.processing, m.record.pending = "ready", true, 1
	m.Update(ingestDoneMsg{err: errors.New("done either way")}) // the entry is finished
	if len(reopened) != 1 {
		t.Fatalf("the app did not start again after the entry: %v", reopened)
	}
}

// A log set to Swedish fetches the Swedish model on its first entry, as the general model is.
func TestSwedishFetchesItsModelOnFirstUse(t *testing.T) {
	need := modelsToFetch(swedishModel.Name)
	if len(need) != 2 || need[0].Name != swedishModel.Name || need[1].Name != speechModels[1].Name {
		t.Fatalf("for Swedish: %v", need)
	}
	if got := modelsToFetch(speechModels[0].Name); len(got) != len(speechModels) {
		t.Fatalf("for the general model: %v", got)
	}
	if modelsToFetch("a-model-of-my-own.bin") != nil {
		t.Fatal("a model Poiesis cannot fetch was offered for fetching")
	}
}

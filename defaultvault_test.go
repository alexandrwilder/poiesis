package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A new log lives in the home folder, outside anything a sync empties or reads. A log made
// before 0.1, in Documents, stays where it is. A log the person chose wins over both, and the
// first log a window opens is remembered, so a later default never moves it.
func TestWhereTheLogLives(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("POIESIS_VAULT", "")
	if got, want := defaultVaultDir(), filepath.Join(home, "Poiesis Vault"); got != want {
		t.Fatalf("a new log: %s, want %s", got, want)
	}
	rememberFirstVault(filepath.Join(home, "Poiesis Vault"))
	if err := os.MkdirAll(appStateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appStateDir(), "streak.txt"), []byte("1 day logged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := defaultVaultDir(), filepath.Join(home, "Poiesis Vault"); got != want {
		t.Fatalf("the first log moved once the app had run: %s, want %s", got, want)
	}

	// a Mac where Poiesis ran before 0.1, without a remembered log: it stays in Documents
	if err := os.Remove(filepath.Join(appStateDir(), "vault.txt")); err != nil {
		t.Fatal(err)
	}
	if got, want := defaultVaultDir(), filepath.Join(home, "Documents", "Poiesis Vault"); got != want {
		t.Fatalf("a log from before 0.1: %s, want %s", got, want)
	}

	// the person's own choice wins
	chosen := filepath.Join(home, "Library", "Mobile Documents", "Poiesis Vault")
	if err := setVaultPointer(chosen); err != nil {
		t.Fatal(err)
	}
	rememberFirstVault(filepath.Join(home, "Poiesis Vault")) // must not overwrite the choice
	if got := defaultVaultDir(); got != chosen {
		t.Fatalf("the chosen log: %s, want %s", got, chosen)
	}
}

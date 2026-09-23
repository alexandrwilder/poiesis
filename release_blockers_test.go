package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The four faults the 16 September audit found before a new person may install: each test
// fails on the code as it was.

// A new vault processes on this computer. Sending text to a cloud AI is only ever the
// person's own choice, and a choice left empty means local too (audit P0-1).
func TestFreshVaultExtractsLocally(t *testing.T) {
	root := t.TempDir()
	if _, err := OpenVault(root); err != nil {
		t.Fatal(err)
	}
	again, err := OpenVault(root) // what was saved, read back
	if err != nil {
		t.Fatal(err)
	}
	if again.Config.Extractor != "ollama" {
		t.Fatalf("a fresh vault extracts with %q, not on this computer", again.Config.Extractor)
	}
	e, err := NewExtractor(Config{Extractor: ""}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, local := e.(*ollamaExtractor); !local {
		t.Fatalf("an empty choice gives %T, not the local AI", e)
	}
}

// When voice detection cannot start, whisper runs again without it. That second run
// working used to crash the app (audit P0-2).
func TestVoiceDetectionRetryThatWorks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script stands in for whisper")
	}
	dir := t.TempDir()
	v, err := OpenVault(filepath.Join(dir, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	models := filepath.Join(dir, "models")
	if err := os.MkdirAll(models, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{v.Config.TurboModel, v.Config.VADModel} {
		if err := os.WriteFile(filepath.Join(models, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fake := filepath.Join(dir, "whisper-cli")
	script := `#!/bin/sh
case " $* " in *" --vad "*) echo "failed to initialize VAD context"; exit 1 ;; esac
while [ $# -gt 0 ]; do [ "$1" = "-of" ] && base="$2"; shift; done
printf '{"result":{"language":"en"},"transcription":[{"text":" hello","offsets":{"from":0,"to":900}}]}' > "$base.json"
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	v.Config.ModelsDir, v.Config.WhisperBin, v.Config.Language = models, fake, "en"
	m := &Media{Path: "raw/2026/09/clip.mp4", DurationS: 1}
	if err := os.MkdirAll(filepath.Dir(v.Path(m.Path)), 0o755); err != nil {
		t.Fatal(err)
	}
	tr, err := Transcribe(v, m, filepath.Join(dir, "clip.wav"), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Words) != 1 || tr.Words[0].Text != "hello" {
		t.Fatalf("the retry's words were lost: %+v", tr.Words)
	}
}

// A reload that fails keeps the log on screen and says why; it used to crash (audit P1-6).
func TestFailedReloadKeepsTheLog(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the streak file belongs to the real app
	data := &tuiData{}
	m := &tuiModel{v: &Vault{Config: defaultConfig()}, data: data}
	next, _ := m.Update(dataReloadedMsg{status: "reload failed: the disk is full"})
	got := next.(*tuiModel)
	if got.data != data {
		t.Fatal("the log on screen was dropped")
	}
	if !strings.Contains(got.status, "reload failed") {
		t.Fatalf("the status does not say what happened: %q", got.status)
	}
}

// A since: filter of any length is safe, also longer than a claim's date (audit P1-7).
func TestSinceFilterOfAnyLength(t *testing.T) {
	d := &tuiData{claims: []Claim{{Text: "early", StatedAt: "2026-09-01"}, {Text: "late", StatedAt: "2026-09-20"}}}
	for _, q := range []string{"since:2026-09-10", "since:2026-09-10-and-much-more-than-a-date"} {
		got := d.searchClaims(q)
		if len(got) != 1 || got[0].Text != "late" {
			t.Fatalf("%q found %+v, want only the late claim", q, got)
		}
	}
}

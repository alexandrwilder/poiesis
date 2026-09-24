package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"
)

// Poiesis links are the one way anything outside the app asks it for something: the menu
// bar, a note, a shortcut, an AI. Anything else is refused.
func TestLinks(t *testing.T) {
	long := strings.Repeat("a", 500)
	for _, c := range []struct {
		in   string
		want link
		bad  bool
	}{
		{in: "record", want: link{record: true}}, // what the menu bar item has always written
		{in: "poiesis://record", want: link{record: true}},
		{in: "poiesis://record?about=the%20launch%2C%20honestly", want: link{record: true, about: "the launch, honestly"}},
		{in: "poiesis://record?about=line%0Atwo", want: link{record: true, about: "line two"}},
		{in: "poiesis://record?about=" + long, want: link{record: true, about: long[:maxAbout]}},
		{in: "poiesis://record?about=" + strings.Repeat("%C3%A5", 130), want: link{record: true, about: strings.Repeat("å", maxAbout)}},
		{in: "poiesis://2026-09-02-a?t=63.2", want: link{entry: "2026-09-02-a", t: 63.2}},
		{in: "poiesis://delete-everything", bad: true},
		{in: "https://example.com/record", bad: true},
		{in: "https://record?about=x", bad: true}, // only Poiesis's own links
		{in: "", bad: true},
	} {
		got, err := parseLink(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("%q was accepted as %+v", c.in, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%q gave %+v, %v; want %+v", c.in, got, err, c.want)
		}
	}
	if l, err := parseLink(recordLink("the launch, honestly")); err != nil || l.about != "the launch, honestly" {
		t.Fatalf("a record link does not read back: %+v, %v", l, err)
	}
}

// countCameraOpens stands in for the camera opener in a test and counts how often it was asked.
func countCameraOpens(t *testing.T) *int {
	opened := 0
	saved := cameraOpener
	cameraOpener = func(*Vault, captureOptions) (camera, error) {
		opened++
		return nil, errors.New("no camera in a test")
	}
	t.Cleanup(func() { cameraOpener = saved })
	return &opened
}

// A record link opens the record screen, ready, with the line to talk about and the camera
// off: the person's first key turns it on, and only the person starts the recording. An entry
// being recorded is never relabelled by a link.
func TestARecordLinkOpensTheRecordScreenReady(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	opened := countCameraOpens(t)
	m := &tuiModel{v: &Vault{Config: defaultConfig()}, data: &tuiData{}, focused: true}
	m.log.search = newInput("") // the log screen, where leaving goes
	m.v.Config.Reflection = "on"
	m.scr = screenLog
	m.follow("poiesis://record?about=the%20launch")
	if m.scr != screenRecord || m.record.about != "the launch" {
		t.Fatalf("screen %v, about %q", m.scr, m.record.about)
	}
	if m.record.phase != "ready" {
		t.Fatalf("a link started the recording itself: phase %q", m.record.phase)
	}
	m.Update(tea.FocusMsg{}) // the window comes to the front
	if *opened != 0 || !m.record.resting {
		t.Fatalf("a link turned the camera on: asked %d times, resting %v", *opened, m.record.resting)
	}
	m.updateRecord(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}) // the person's first key
	if *opened != 1 || m.record.resting || m.record.phase != "ready" {
		t.Fatalf("the first key: asked %d times, resting %v, phase %q", *opened, m.record.resting, m.record.phase)
	}
	m.record.phase = "recording"
	m.follow("poiesis://record?about=something%20else")
	if m.record.about != "the launch" {
		t.Fatalf("a link relabelled an entry being recorded: %q", m.record.about)
	}
	m.record.phase = "ready"
	m.leaveRecord()
	if m.record.about != "" {
		t.Fatalf("the line to talk about outlived the record screen: %q", m.record.about)
	}
}

// A window that a link started opens with the camera off too.
func TestAWindowOpenedByALinkStartsWithTheCameraOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	opened := countCameraOpens(t)
	if err := os.MkdirAll(appStateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(commandFile(), []byte("poiesis://record?about=x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &tuiModel{v: &Vault{Root: t.TempDir(), Config: defaultConfig()}, data: &tuiData{}, focused: true}
	m.v.Config.Reflection = "on"
	m.Init()
	if m.scr != screenRecord || !m.record.resting || *opened != 0 {
		t.Fatalf("screen %v, resting %v, camera asked %d times", m.scr, m.record.resting, *opened)
	}
	if _, err := os.Stat(commandFile()); err != nil {
		t.Fatalf("the window took the link before following it: %v", err)
	}
}

// A link that no window took within a minute is dropped, so it cannot label a later entry.
func TestAStaleLinkIsDropped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "command")
	now := time.Now()
	if err := os.WriteFile(path, []byte("poiesis://record?about=x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := takeCommand(path, now); got != "poiesis://record?about=x" {
		t.Fatalf("a fresh link gave %q", got)
	}
	if err := os.WriteFile(path, []byte("poiesis://record?about=x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := takeCommand(path, now.Add(2*time.Minute)); got != "" {
		t.Fatalf("a stale link was followed: %q", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the stale link stayed in the command file")
	}
}

// The AI's record tool opens Poiesis ready, and nothing more.
func TestTheRecordToolOnlyOpensTheRecordScreen(t *testing.T) {
	var opened string
	saved := openLink
	openLink = func(root, l string) error { opened = l; return nil }
	defer func() { openLink = saved }()
	v := &Vault{Root: t.TempDir()}
	msg, err := v.mcpRecord(recordIn{About: "the launch"})
	if err != nil {
		t.Fatal(err)
	}
	if l, err := parseLink(opened); err != nil || !l.record || l.about != "the launch" {
		t.Fatalf("the tool opened %q", opened)
	}
	if !strings.Contains(msg, "space") {
		t.Fatalf("the tool does not tell the person how to start: %q", msg)
	}
}

// An entry keeps what it set out to be about, and says nothing when there was nothing.
func TestAnEntryKeepsWhatItWasAbout(t *testing.T) {
	with, _ := yaml.Marshal(Episode{ID: "2026-09-24-a", Prompt: "the launch"})
	without, _ := yaml.Marshal(Episode{ID: "2026-09-24-b"})
	if !strings.Contains(string(with), "prompt: the launch") || strings.Contains(string(without), "prompt") {
		t.Fatalf("with:\n%s\nwithout:\n%s", with, without)
	}
}

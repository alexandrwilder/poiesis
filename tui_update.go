package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// While the app is open it looks for a newer version every hour, when the setting allows it,
// and says so in the frame. One look is a single small request (about 5 KB, a tenth of a
// second, measured), so releases several times a day reach people within the hour. Installing it is the person's choice: enter on the updates row in
// settings. Never during an entry: the app starts again afterwards.

type updateFoundMsg struct{ version string }

type updateNoneMsg struct{ err error } // a look asked for by hand that found nothing newer

type updateTickMsg struct{}

type updateInstalledMsg struct {
	version string
	reopen  string // the app to open again, when the app was replaced
	note    string // what to do instead, when another tool updates this copy
	err     error
}

const updateEvery = time.Hour

// updateCheckDue: the setting allows a look (anything but "off"; older copies wrote
// "daily"), and the last look was most of an hour ago.
func updateCheckDue(c Config) bool {
	if c.UpdateCheck == "off" {
		return false
	}
	st, err := os.Stat(filepath.Join(appStateDir(), "update-checked"))
	return err != nil || time.Since(st.ModTime()) > updateEvery-10*time.Minute
}

// checkUpdateCmd asks for the newest version in the background. A daily look stays quiet
// when there is nothing newer or no release page to reach; one asked for by hand reports.
func checkUpdateCmd(byHand bool) tea.Cmd {
	return func() tea.Msg {
		v, err := latestVersion()
		_ = os.MkdirAll(appStateDir(), 0o755)
		_ = os.WriteFile(filepath.Join(appStateDir(), "update-checked"), []byte(time.Now().Format(time.RFC3339)+"\n"), 0o644)
		if err == nil && newerVersion(v, version) {
			return updateFoundMsg{v}
		}
		if byHand {
			return updateNoneMsg{err}
		}
		return nil
	}
}

// startUpdateChecks looks now when a look is due, and again every hour the app stays open.
func (m *tuiModel) startUpdateChecks() tea.Cmd {
	tick := tea.Tick(updateEvery, func(time.Time) tea.Msg { return updateTickMsg{} })
	if updateCheckDue(m.v.Config) {
		return tea.Batch(tick, checkUpdateCmd(false))
	}
	return tick
}

// entryUnderway: an entry is being recorded, paused or processed.
func (m *tuiModel) entryUnderway() bool {
	return m.record.processing || m.record.phase == "recording" || m.record.phase == "paused"
}

// installUpdate is enter on the updates row: look now, or install what was found.
func (m *tuiModel) installUpdate() tea.Cmd {
	if m.record.processing || m.record.phase == "recording" || m.record.phase == "paused" {
		m.status = "the update waits until this entry is done"
		return nil
	}
	if m.update == "" {
		m.status = "looking for a newer version…"
		return checkUpdateCmd(true)
	}
	ver := m.update
	m.status = "downloading Poiesis " + ver + "…"
	return func() tea.Msg {
		exe, err := os.Executable()
		if err != nil {
			return updateInstalledMsg{err: err}
		}
		quiet := func(string, ...any) {}
		switch way, target := howInstalled(exe); way {
		case byMacApp:
			if err := updateMacApp(target, ver, quiet); err != nil {
				return updateInstalledMsg{err: err}
			}
			return updateInstalledMsg{version: ver, reopen: target}
		case byHomeBinary:
			if err := updateBinary(target, ver, quiet); err != nil {
				return updateInstalledMsg{err: err}
			}
			return updateInstalledMsg{version: ver, note: "Poiesis " + ver + " is installed: quit and start it again"}
		case byHomebrew:
			return updateInstalledMsg{note: "Poiesis " + ver + " is out: brew upgrade poiesis"}
		case byPackageManager:
			return updateInstalledMsg{note: "Poiesis " + ver + " is out: update it with your package manager"}
		default:
			return updateInstalledMsg{note: "Poiesis " + ver + " is out: git pull, then go build"}
		}
	}
}

// updateMsg handles the messages above; ok is false for any other message.
func (m *tuiModel) updateMsg(msg tea.Msg) (cmd tea.Cmd, ok bool) {
	switch msg := msg.(type) {
	case updateFoundMsg:
		m.update = msg.version
	case updateNoneMsg:
		if msg.err != nil {
			m.status = "could not reach the release page: " + msg.err.Error()
		} else {
			m.status = "Poiesis " + version + " is the newest"
		}
	case updateTickMsg:
		return m.startUpdateChecks(), true
	case updateInstalledMsg:
		switch {
		case msg.err != nil:
			m.status = "the update did not happen: " + msg.err.Error()
		case msg.reopen != "" && m.entryUnderway():
			m.reopenAfter = msg.reopen // an entry is never cut short by an update
			m.status = "Poiesis " + msg.version + " is installed · it starts again when this entry is done"
		case msg.reopen != "":
			m.status = "Poiesis " + msg.version + " is installed · starting it again"
			reopenApp(msg.reopen)
			return tea.Quit, true
		default:
			m.status = msg.note
			if msg.version != "" {
				m.update = ""
			}
		}
	default:
		return nil, false
	}
	return nil, true
}

// reopenApp opens the app again a moment after this one has quit, from a process of its own.
// reopenApp starts the app again after an update; a test puts a fake in its place.
var reopenApp = func(app string) {
	c := exec.Command("/bin/sh", "-c", `sleep 1; open "$0"`, app)
	detach(c)
	_ = c.Start()
}

// frameNote is the text in the frame's top right: the streak, and a newer version when
// there is one.
func (m *tuiModel) frameNote() string {
	s := m.streakText(false)
	if m.update != "" {
		s += "  ·  " + m.update + " is ready (settings)"
	}
	return s
}

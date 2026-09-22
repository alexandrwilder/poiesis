package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Settings: what goes in, what comes out, where it lives, and how you look on screen.
// Up and down choose a row, left and right change it, enter edits the folder. Every change
// is saved at once. The badge at the top says whether anything leaves this computer.

type settingsState struct {
	cursor int
	video  []avDevice
	audio  []avDevice
	loaded bool
	path   textinput.Model
	moving bool
}

var languages = []string{"auto", "sv", "en"}
var videoModes = []string{"clear", "light", "styled"}

const (
	rowTheme = iota
	rowVideo
	rowCamera
	rowMic
	rowExtract
	rowLanguage
	rowLimit
	rowVault
	rowAI
	rowCount
)

func (m *tuiModel) enterSettings() {
	m.scr = screenSettings
	st := &m.settings
	if !st.loaded {
		st.video, st.audio = listCaptureDevices(findTool(m.v.Config.FFmpegBin))
		st.loaded = true
	}
	if st.path.Placeholder == "" {
		st.path = newInput("folder for your log")
	}
}

// pictureStyle keeps older settings meaningful: anything but "off" means the camera is on.
func pictureStyle(s string) string {
	if s == "off" {
		return "off"
	}
	return "on"
}

func (m *tuiModel) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	st := &m.settings
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if st.moving {
		switch key.String() {
		case "enter":
			st.moving = false
			st.path.Blur()
			return m, m.moveVault(strings.TrimSpace(st.path.Value()))
		case "esc":
			st.moving = false
			st.path.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		st.path, cmd = st.path.Update(msg)
		return m, cmd
	}
	c := &m.v.Config
	switch key.String() {
	case "esc", "tab", "s", "q":
		return m, m.enterRecord()
	case "up", "k":
		if st.cursor > 0 {
			st.cursor--
		}
	case "down", "j":
		if st.cursor < rowCount-1 {
			st.cursor++
		}
	case "enter":
		if st.cursor == rowAI {
			return m, m.connectAI()
		}
		if st.cursor == rowVault {
			st.moving = true
			st.path.SetValue(m.v.Root)
			st.path.Focus()
			return m, nil
		}
		return m.updateSettings(tea.KeyMsg{Type: tea.KeyRight})
	case "left", "right":
		dir := 1
		if key.String() == "left" {
			dir = -1
		}
		switch st.cursor {
		case rowCamera:
			cam, mic := splitDevice(c.CaptureDevice)
			c.CaptureDevice = joinDevice(stepDevice(st.video, cam, dir), mic)
			m.stopCap() // the next record screen opens the new camera
		case rowMic:
			cam, mic := splitDevice(c.CaptureDevice)
			c.CaptureDevice = joinDevice(cam, stepDevice(st.audio, mic, dir))
			m.stopCap()
		case rowTheme:
			c.Theme = step(themeNames(), themeByName(c.Theme).Name, dir)
			applyTheme(c.Theme)
		case rowVideo:
			c.Video = step(videoModes, videoMode(*c), dir)
			m.stopCap() // the camera stream is sized for the mode
		case rowExtract:
			if c.Extractor == "ollama" {
				c.Extractor = "claude"
			} else {
				c.Extractor = "ollama"
			}
		case rowLanguage:
			c.Language = step(languages, c.Language, dir)
		case rowLimit:
			s := c.MaxEntryS
			if s <= 0 {
				s = 180
			}
			s += 30 * dir
			if s < 30 {
				s = 30
			}
			if s > 600 {
				s = 600
			}
			c.MaxEntryS = s
		case rowVault, rowAI:
			return m, nil
		}
		if err := m.v.SaveConfig(); err != nil {
			m.status = "could not save: " + err.Error()
		} else {
			m.status = "saved"
		}
	}
	return m, nil
}

func step(list []string, cur string, dir int) string {
	i := 0
	for j, v := range list {
		if v == cur {
			i = j
		}
	}
	i = ((i+dir)%len(list) + len(list)) % len(list)
	return list[i]
}

func stepDevice(list []avDevice, cur, dir int) int {
	if len(list) == 0 {
		n := cur + dir
		if n < 0 {
			n = 0
		}
		return n
	}
	i := 0
	for j, d := range list {
		if d.Index == cur {
			i = j
		}
	}
	i = ((i+dir)%len(list) + len(list)) % len(list)
	return list[i].Index
}

// moveVault points Poiesis at another folder, moving the current one there when the target
// does not exist yet. The pointer is written for every later launch.
func (m *tuiModel) moveVault(target string) tea.Cmd {
	if target == "" {
		return nil
	}
	if strings.HasPrefix(target, "~/") {
		home, _ := os.UserHomeDir()
		target = filepath.Join(home, target[2:])
	}
	target = filepath.Clean(target)
	if target == filepath.Clean(m.v.Root) {
		m.status = "that is where it is"
		return nil
	}
	m.stopCap()
	if _, err := os.Stat(target); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			m.status = "cannot create that: " + err.Error()
			return nil
		}
		if err := os.Rename(m.v.Root, target); err != nil {
			m.status = "could not move the folder: " + err.Error()
			return nil
		}
	}
	v, err := OpenVault(target)
	if err != nil {
		m.status = "could not open " + target + ": " + err.Error()
		return nil
	}
	if err := setVaultPointer(target); err != nil {
		m.status = "moved, but could not remember it: " + err.Error()
	}
	m.v = v
	return reloadCmd(m.v, "your log now lives in "+target+"  (run `poiesis setup --mcp` again for the AI connection)", "")
}

// setVaultPointer remembers the vault for every way Poiesis can start.
func setVaultPointer(root string) error {
	if err := os.MkdirAll(appStateDir(), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(appStateDir(), "vault.txt"), []byte(root+"\n"), 0o644); err != nil {
		return err
	}
	if self, err := os.Executable(); err == nil && inMacApp() {
		_ = os.WriteFile(filepath.Join(filepath.Dir(filepath.Dir(self)), "Resources", "vault.txt"), []byte(root+"\n"), 0o644)
	}
	return nil
}

func (m *tuiModel) viewSettings() string {
	st := &m.settings
	c := m.v.Config
	var b strings.Builder
	// the badge: what leaves this computer
	if c.Extractor == "claude" {
		b.WriteString(sAmber.Render("▲ your key") + sDim.Render("   the words of each entry are sent to Anthropic under your own key. video and sound never leave.") + "\n\n")
	} else {
		b.WriteString(sOK.Render("● local") + sDim.Render("   nothing leaves this computer. the speech model and the local AI run inside the app.") + "\n\n")
	}
	cam, mic := splitDevice(c.CaptureDevice)
	limit := c.MaxEntryS
	if limit <= 0 {
		limit = 180
	}
	extract := "local AI  (inside the app, " + ollamaModelName(c) + ")"
	if c.Extractor == "claude" {
		extract = "your own Claude key"
	}
	t := themeByName(c.Theme)
	video := videoMode(c)
	videoNote := "clear: sharp, 18 a second · light: softer, 10 a second, half the cost · styled: drawn in cells, cheapest"
	if video == "clear" && !terminalDrawsImages() {
		videoNote = "this terminal cannot draw pixels: the styled picture is used"
	}
	rows := []struct{ label, value, note string }{
		{"theme", t.Name, t.Note},
		{"video", video, videoNote},
		{"camera", deviceName(st.video, cam), ""},
		{"microphone", deviceName(st.audio, mic), ""},
		{"extraction", extract, "local: the words never leave · key: the words go to Anthropic, under your account"},
		{"language", c.Language, "auto · sv · en"},
		{"entry limit", mmss(float64(limit)), "an entry completes itself here"},
		{"log folder", m.v.Root, "enter to move it"},
		{"your AI apps", aiStatus(), "enter connects Claude Code and writes CONNECT.md in your log for Desktop and Cursor"},
	}
	for i, r := range rows {
		cur, lab, val := "  ", sDim, sInk
		if i == st.cursor {
			cur, lab, val = sCursor.Render("▶ "), sMid, sAmber
		}
		value := r.value
		if i == rowVault && st.moving {
			value = st.path.View()
		}
		line := cur + lab.Render(padRight(r.label, 13)) + val.Render(value)
		if r.note != "" && i == st.cursor {
			line += sDim.Render("   " + r.note)
		}
		b.WriteString(cut(line, m.inner()) + "\n")
	}
	if st.moving {
		b.WriteString("\n" + sDim.Render("  type the folder, enter to move, esc to keep it. an existing empty folder is used as is.") + "\n")
	}
	b.WriteString("\n" + sDim.Render("  everything here is saved as you change it, in config.json inside your log folder."))
	return b.String()
}

func ollamaModelName(c Config) string {
	if c.Model != "" {
		return c.Model
	}
	return setupOllamaModel
}

// settings rows can be clicked too
func (m *tuiModel) clickSettings(y int) (tea.Model, tea.Cmd) {
	i := y - headerRows - 2 // the badge line and a blank line
	if i < 0 || i >= rowCount {
		return m, nil
	}
	if m.settings.cursor == i {
		return m.updateSettings(tea.KeyMsg{Type: tea.KeyRight})
	}
	m.settings.cursor = i
	return m, nil
}

var _ = fmt.Sprintf

// videoMode is the setting, or a choice made for a struggling machine when unset.
func videoMode(c Config) string {
	switch c.Video {
	case "styled", "light", "clear":
		return c.Video
	}
	if machineStruggles() {
		return "light"
	}
	return "clear"
}

// aiStatus says whether Claude Code knows this log.
func aiStatus() string {
	if _, err := exec.LookPath("claude"); err != nil {
		return "not connected  (Claude Code not installed; CONNECT.md has the lines for other apps)"
	}
	out, _ := exec.Command("claude", "mcp", "get", "poiesis").CombinedOutput()
	if strings.Contains(string(out), "poiesis") && !strings.Contains(strings.ToLower(string(out)), "not found") {
		return "Claude Code connected"
	}
	return "not connected  (enter)"
}

// connectAI registers the log with Claude Code and leaves the lines for the other apps.
func (m *tuiModel) connectAI() tea.Cmd {
	lines := connectText(m.v)
	_ = os.WriteFile(m.v.Path("CONNECT.md"), []byte("# Connect your AI to this log\n\n```\n"+lines+"\n```\n"), 0o644)
	if err := registerMCP(m.v, func(string, ...any) {}); err != nil {
		m.status = "wrote CONNECT.md in your log · " + err.Error()
		return nil
	}
	m.status = "Claude Code connected · CONNECT.md in your log has the lines for Desktop and Cursor"
	return nil
}

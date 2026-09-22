package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The terminal app: one model, a handful of screens, the log aesthetic.
// Black ground, warm text, one amber, one green. Mono is the terminal itself.

type screen int

const (
	screenRecord screen = iota
	screenLog
	screenEntry
	screenEntities
	screenEntity
	screenSettings
	screenAsk
)

var (
	cAmber = lipgloss.Color("#E8A33D")
	cInk   = lipgloss.Color("#E6E4DC")
	cMid   = lipgloss.Color("#A7AAB0")
	cDim   = lipgloss.Color("#6F747C")
	cOK    = lipgloss.Color("#7DBE8A")
	cRule  = lipgloss.Color("#3A3F47")

	sInk     = lipgloss.NewStyle().Foreground(cInk)
	sMid     = lipgloss.NewStyle().Foreground(cMid)
	sDim     = lipgloss.NewStyle().Foreground(cDim)
	sAmber   = lipgloss.NewStyle().Foreground(cAmber)
	sAmberB  = lipgloss.NewStyle().Foreground(cAmber).Bold(true)
	sOK      = lipgloss.NewStyle().Foreground(cOK)
	sHead    = lipgloss.NewStyle().Foreground(cInk).Bold(true)
	sCursor  = lipgloss.NewStyle().Foreground(cAmber)
	sRule    = lipgloss.NewStyle().Foreground(cRule)
	sKey     = lipgloss.NewStyle().Foreground(cInk).Background(lipgloss.Color("#2A2E35"))
	sAccent2 = lipgloss.NewStyle().Foreground(cAmber)
)

type tuiModel struct {
	v          *Vault
	data       *tuiData
	scr        screen
	width      int
	height     int
	status     string // one line at the bottom, transient
	startOnLog bool   // open on the log page instead of the record screen (log_ ui --log)
	fullWindow bool   // the last record view painted the whole window itself
	focused    bool   // the window is in front; when it is not, the camera rests

	entries  listState
	entry    entryState
	entities listState
	entity   entityState
	log      logState
	record   recordState
	settings settingsState
	ask      askState
}

type listState struct {
	cursor int
	offset int
}

func newTUI(v *Vault) (*tuiModel, error) {
	d, err := loadTUIData(v)
	if err != nil {
		return nil, err
	}
	applyTheme(v.Config.Theme)
	m := &tuiModel{v: v, data: d, width: 100, height: 30, focused: true}
	m.log.search = newInput("what was said, a name, a date")
	m.record.mission = newInput("mission, or empty for free run")
	m.record.mission.SetValue(v.Config.LastMission)
	return m, nil
}

func (m *tuiModel) Init() tea.Cmd {
	writeWindowPID()
	m.cacheStreak()
	if m.startOnLog {
		m.enterLog()
		return watchCommandCmd()
	}
	// the record screen is the front door: open it, ready, camera faint
	return tea.Batch(m.enterRecord(), watchCommandCmd())
}

// writeWindowPID lets the menu bar item find this window instead of opening another.
func writeWindowPID() {
	_ = os.MkdirAll(appStateDir(), 0o755)
	_ = os.WriteFile(windowPIDFile(), []byte(fmt.Sprint(os.Getpid())), 0o644)
}

func clearWindowPID() {
	if b, err := os.ReadFile(windowPIDFile()); err == nil && strings.TrimSpace(string(b)) == fmt.Sprint(os.Getpid()) {
		_ = os.Remove(windowPIDFile())
	}
}

// externalCmdMsg is a line the menu bar item left for this window.
type externalCmdMsg string

func watchCommandCmd() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg {
		b, err := os.ReadFile(commandFile())
		if err != nil {
			return externalCmdMsg("")
		}
		_ = os.Remove(commandFile())
		return externalCmdMsg(strings.TrimSpace(string(b)))
	})
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.FocusMsg:
		m.focused = true
		if m.scr == screenRecord && m.record.phase == "ready" && m.reflectionOn() {
			m.startPreview() // back in front: the camera comes back
		}
		return m, nil
	case tea.BlurMsg:
		m.focused = false
		if m.scr == screenRecord && m.record.phase == "ready" {
			m.stopCap() // nobody is looking: the camera and the picture rest
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// the camera stream has a fixed size; the picture is sampled to the new window at once
		return m, nil
	case dataReloadedMsg:
		m.data = msg.data
		m.status = msg.status
		m.cacheStreak()
		if msg.openEntry != "" {
			for i, e := range m.data.entries {
				if e.ID == msg.openEntry {
					m.entries.cursor = i
					m.openEntry(i)
					break
				}
			}
		}
		return m, nil
	case tickMsg:
		// nothing for three minutes while ready: the camera rests until a key
		if m.scr == screenRecord && m.record.phase == "ready" && m.record.cap != nil && !m.record.resting &&
			!m.record.lastKey.IsZero() && time.Since(m.record.lastKey) > 3*time.Minute {
			m.stopCap()
			m.record.resting = true
			m.status = "camera resting · any key wakes it"
		}
		// a camera that stops on its own (blocked, unplugged) must say so, not go black
		if c := m.record.cap; c != nil && m.record.phase == "ready" {
			select {
			case err := <-c.done:
				m.record.cap = nil
				m.record.lastErr = cameraProblem(err)
			default:
			}
		}
		// an entry completes itself at the limit, so a forgotten recording cannot run on
		if m.record.phase == "recording" && m.totalRecorded() >= m.maxEntry() {
			m.status = fmt.Sprintf("%s reached · the entry completed itself", mmss(m.maxEntry().Seconds()))
			return m, m.completeEntry()
		}
		if m.scr == screenRecord && m.record.cap != nil && m.reflectionOn() && m.focused {
			return m, m.fastTickCmd() // a picture is showing: follow the camera, as fast as the machine allows
		}
		if m.scr == screenRecord || m.record.processing || (m.scr == screenAsk && m.ask.asking) {
			return m, tickCmd()
		}
		return m, nil
	case ingestDoneMsg:
		m.record.processing = false
		m.record.completed = ""
		if msg.err != nil {
			if errors.Is(msg.err, ErrNoSpeech) {
				m.record.lastErr = ""
				m.status = "nothing was said in that clip, so no entry was made. the video is kept in raw/"
				return m, nil
			}
			m.record.lastErr = "processing failed: " + msg.err.Error()
			m.status = m.record.lastErr
			return m, nil
		}
		return m, reloadCmd(m.v, fmt.Sprintf("entry %s ready: %d claims", msg.ep.ID, msg.ep.ClaimCount), msg.ep.ID)
	case askDoneMsg:
		return m.updateAsk(msg)
	case externalCmdMsg:
		if msg == "record" && m.scr != screenRecord {
			return m, tea.Batch(m.enterRecord(), watchCommandCmd())
		}
		return m, watchCommandCmd()
	case tea.MouseMsg:
		return m.updateMouse(msg)
	case tea.KeyMsg:
		traceKey(msg)
		if msg.String() == "ctrl+c" {
			m.stopCaptureIfRunning()
			clearWindowPID()
			stopOllama()
			return m, tea.Quit
		}
	}
	switch m.scr {
	case screenLog:
		return m.updateLog(msg)
	case screenEntry:
		return m.updateEntry(msg)
	case screenEntities:
		return m.updateEntities(msg)
	case screenEntity:
		return m.updateEntity(msg)
	case screenRecord:
		return m.updateRecord(msg)
	case screenSettings:
		return m.updateSettings(msg)
	case screenAsk:
		return m.updateAsk(msg)
	}
	return m, nil
}

func (m *tuiModel) View() string {
	var body string
	switch m.scr {
	case screenLog:
		body = m.viewLog()
	case screenEntry:
		body = m.viewEntry()
	case screenEntities:
		body = m.viewEntities()
	case screenEntity:
		body = m.viewEntity()
	case screenRecord:
		body = m.viewRecord()
	case screenSettings:
		body = m.viewSettings()
	case screenAsk:
		body = m.viewAsk()
	}
	if m.scr == screenRecord && m.fullWindow {
		return body // the picture fills the window; the frame is painted over it
	}
	inner := m.inner()
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	var b strings.Builder
	b.WriteString(windowTintReset + kittyClear + m.frameTop(inner) + "\n")
	for i := 0; i < m.bodyHeight(); i++ {
		line := ""
		if i < len(lines) {
			line = ansi.Truncate(lines[i], inner, "…")
		}
		pad := inner - lipgloss.Width(line)
		if pad < 0 {
			pad = 0
		}
		b.WriteString(sRule.Render("│") + " " + line + strings.Repeat(" ", pad) + " " + sRule.Render("│") + "\n")
	}
	b.WriteString(m.frameBottom(inner))
	return b.String()
}

// inner is the number of columns inside the frame.
func (m *tuiModel) inner() int { return max(20, m.width-4) }

// frameTop: ╭──────────────────────────── 12 days in a row · 47 days logged ─╮
func (m *tuiModel) frameTop(inner int) string {
	right := sRule.Render(" ") + sAccent2.Render(m.streakText(false)) + sRule.Render(" ─")
	fill := inner + 2 - lipgloss.Width(right)
	if fill < 1 {
		fill = 1
	}
	return sRule.Render("╭") + sRule.Render(strings.Repeat("─", fill)) + right + sRule.Render("╮")
}

// frameBottom: ╰─ [enter] open  [r] record … ─────────────── status ─╯
func (m *tuiModel) frameBottom(inner int) string {
	keys := m.keys()
	if m.status != "" {
		keys = cut(keys, max(10, inner-lipgloss.Width(m.status)-6))
	} else {
		keys = cut(keys, max(10, inner-4))
	}
	left := sRule.Render("─ ") + sDim.Render(keys) + sRule.Render(" ")
	right := sRule.Render("─")
	if m.status != "" {
		right = sRule.Render(" ") + sAmber.Render(m.status) + sRule.Render(" ─")
	}
	fill := inner + 2 - lipgloss.Width(left) - lipgloss.Width(right)
	if fill < 1 {
		fill = 1
	}
	return sRule.Render("╰") + left + sRule.Render(strings.Repeat("─", fill)) + right + sRule.Render("╯")
}

// keyHint is one thing you can do here: the key, then what it does.
type keyHint struct{ key, label string }

func hintsOf(pairs ...string) []keyHint {
	var out []keyHint
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, keyHint{pairs[i], pairs[i+1]})
	}
	return out
}

// hints lists the keys for the current screen. An empty key is a plain remark.
func (m *tuiModel) hints() []keyHint {
	switch m.scr {
	case screenLog:
		return hintsOf("↑↓", "choose", "enter", "open", "← →", "mission", "", "type to search", "esc", "clear", "tab", "record")
	case screenEntry:
		if m.entry.mission.open {
			return hintsOf("↑↓", "choose", "enter", "set", "n", "new", "esc", "cancel")
		}
		return hintsOf("enter", "play", "p", "play all", "space", "earlier", "e", "person", "M", "mission", "x", "trash", "a", "ask", "esc", "back")
	case screenAsk:
		if m.ask.typing {
			return hintsOf("", "type your question", "enter", "ask", "esc", "cancel")
		}
		return hintsOf("↑↓", "choose", "enter", "ask", "c", "copy for your AI", "esc", "back")
	case screenSettings:
		if m.settings.moving {
			return hintsOf("", "type the folder", "enter", "move", "esc", "keep")
		}
		return hintsOf("↑↓", "choose", "← →", "change", "enter", "edit", "esc", "back")
	case screenEntities:
		return hintsOf("enter", "open", "esc", "back")
	case screenEntity:
		return hintsOf("enter", "play", "esc", "back")
	case screenRecord:
		switch {
		case m.record.phase == "recording":
			return hintsOf("space", "pause", "enter", "complete", "x", "discard")
		case m.record.phase == "paused":
			return hintsOf("space", "resume", "enter", "complete", "x", "discard")
		case m.record.picker.open && m.record.picker.typing:
			return hintsOf("", "type the new mission's name", "enter", "tag", "esc", "cancel")
		case m.record.picker.open:
			return hintsOf("↑↓", "choose", "enter", "tag the next entries", "n", "new", "esc", "cancel")
		default:
			cam := "camera off"
			if !m.reflectionOn() {
				cam = "camera on"
			}
			return hintsOf("space", "start", "m", "mission", "a", "ask", "v", cam, "s", "settings", "tab", "the log")
		}
	}
	return nil
}

// keys renders the hints as key caps: the key on a small dark cap, the label beside it.
func (m *tuiModel) keys() string {
	var parts []string
	for _, h := range m.hints() {
		if h.key == "" {
			parts = append(parts, sDim.Render(h.label))
			continue
		}
		parts = append(parts, sKey.Render(" "+h.key+" ")+" "+sDim.Render(h.label))
	}
	return strings.Join(parts, "   ")
}

// bodyHeight is the number of rows inside the frame.
func (m *tuiModel) bodyHeight() int {
	h := m.height - 2
	if h < 5 {
		h = 5
	}
	return h
}

// visible keeps the cursor inside a window of n rows.
func (ls *listState) visible(total, n int) (int, int) {
	if ls.cursor < 0 {
		ls.cursor = 0
	}
	if ls.cursor >= total && total > 0 {
		ls.cursor = total - 1
	}
	if ls.cursor < ls.offset {
		ls.offset = ls.cursor
	}
	if ls.cursor >= ls.offset+n {
		ls.offset = ls.cursor - n + 1
	}
	end := ls.offset + n
	if end > total {
		end = total
	}
	return ls.offset, end
}

func moveCursor(ls *listState, key string, total int) bool {
	switch key {
	case "up", "k":
		if ls.cursor > 0 {
			ls.cursor--
		}
		return true
	case "down", "j":
		if ls.cursor < total-1 {
			ls.cursor++
		}
		return true
	case "pgup":
		ls.cursor -= 10
		if ls.cursor < 0 {
			ls.cursor = 0
		}
		return true
	case "pgdown":
		ls.cursor += 10
		if ls.cursor > total-1 {
			ls.cursor = total - 1
		}
		return true
	case "home", "g":
		ls.cursor = 0
		return true
	case "end", "G":
		ls.cursor = total - 1
		return true
	}
	return false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// cut shortens a line that carries colours. fit() counts every character, including the
// invisible colour codes, which is why styled rows came out garbled.
func cut(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 1 {
		return string(r[:w])
	}
	return string(r[:w-1]) + "…"
}

// cameraProblem turns ffmpeg's exit into one readable line.
func cameraProblem(err error) string {
	if err == nil {
		return "the camera stopped"
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "not permitted") || strings.Contains(low, "denied") || strings.Contains(low, "permission"):
		return "the camera is blocked: allow LOG_ in System Settings › Privacy & Security › Camera, then press v"
	case strings.Contains(low, "could not find") || strings.Contains(low, "no such"):
		return "no camera found: check the capture device in config.json"
	}
	if len(msg) > 160 {
		msg = msg[len(msg)-160:]
	}
	return "camera: " + msg
}

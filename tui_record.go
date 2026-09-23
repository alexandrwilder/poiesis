package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The record screen. One entry is one clip, but life interrupts: space pauses and
// resumes inside the same entry (each stretch is a part; parts are joined without
// re-encoding when the entry is completed). Enter completes the entry and sends it to be
// processed; x, twice, discards it. The faint reflection sits under the readout.

type recordState struct {
	mission        textinput.Model
	picker         missionPicker
	phase          string // ready | recording | paused
	cap            camera // ffmpeg or a host (camera.go)
	parts          []string
	recorded       time.Duration // finished parts
	confirmDiscard bool
	processing     bool
	procStart      time.Time
	completed      string // "DAY 0412 · 03:41" shown after completing, until the entry appears
	sentFrame      int    // the camera frame last sent as a picture, and the window it was sent for
	sentW, sentH   int
	lastSent       []byte // the pixels last sent, to tell a still scene from a moving one
	lastKey        time.Time
	resting        bool // the camera stopped because nothing happened for a while
	lastTick       time.Time
	slow           int // how far behind the screen is falling; above 5 the picture slows down
	lastErr        string
}

type tickMsg time.Time

type ingestDoneMsg struct {
	ep  *Episode
	err error
}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// fastTickCmd keeps up with the camera while a picture is on screen. When the machine
// cannot keep that pace, the tick stretches: the picture slows before anything else does.
func (m *tuiModel) fastTickCmd() tea.Cmd {
	st := &m.record
	want := m.frameInterval()
	if !st.lastTick.IsZero() {
		gap := time.Since(st.lastTick)
		if gap > want*2 {
			if st.slow < 12 {
				st.slow++
			}
		} else if st.slow > 0 {
			st.slow--
		}
	}
	st.lastTick = time.Now()
	if st.slow > 5 {
		want = 125 * time.Millisecond // eight frames a second is still a moving picture
	}
	return tea.Tick(want, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *tuiModel) reflectionOn() bool { return pictureStyle(m.v.Config.Reflection) != "off" }

// previewSize is the size of the camera stream behind the reflection. It is fixed and
// generous, so the picture is only ever sampled to the window: a resize never touches the
// camera and the picture follows the window at once.
func (m *tuiModel) previewSize() (int, int) {
	switch {
	case m.clearVideo() && videoMode(m.v.Config) == "light":
		return 480, 270
	case m.clearVideo():
		return 640, 360
	}
	return 256, 144
}

// previewFPS: how many frames a second the camera delivers to the screen. Every frame costs
// the window a copy, a colour conversion, a new texture and a full redraw (Ghostty 1.3), so
// the clear picture runs at 12: measured 2026-09-04, about 45% of a core against 58% at 18.
func (m *tuiModel) previewFPS() int {
	switch videoMode(m.v.Config) {
	case "light", "styled":
		return 10
	}
	return 12
}

// frameInterval is the screen's tick while a picture shows: one tick per camera frame.
func (m *tuiModel) frameInterval() time.Duration {
	return time.Second / time.Duration(m.previewFPS())
}

// clearVideo: real pixels under the text, when the setting and the terminal allow it.
func (m *tuiModel) clearVideo() bool {
	mode := videoMode(m.v.Config)
	return (mode == "clear" || mode == "light") && terminalDrawsImages()
}

// pictureMayStart says whether the camera picture may start now. It rests while an entry
// is processed: the speech model and the local AI fill the graphics chip for a minute or
// more, and a live picture drawn on the same chip is what made the app lag after enter.
func (m *tuiModel) pictureMayStart() bool {
	return m.reflectionOn() && m.record.cap == nil && !m.record.processing
}

func (m *tuiModel) startPreview() {
	if !m.pictureMayStart() {
		return
	}
	w, h := m.previewSize()
	if c, err := openCamera(m.v, captureOptions{PreviewW: w, PreviewH: h, PreviewFPS: m.previewFPS()}); err == nil {
		m.record.cap = c
	} else {
		m.record.lastErr = "no preview: " + err.Error()
	}
}

func (m *tuiModel) stopCap() {
	if m.record.cap != nil {
		_ = m.record.cap.stop()
		m.record.cap = nil
	}
}

func (m *tuiModel) enterRecord() tea.Cmd {
	m.scr = screenRecord
	m.record.lastKey = time.Now()
	m.record.resting = false
	m.record.mission.Blur()
	if m.record.phase == "" {
		m.record.phase = "ready"
	}
	// the camera is on while this screen is open (unless turned off with v), and stops
	// when you tab to the log
	if m.record.phase == "ready" || m.record.phase == "paused" {
		m.startPreview()
	}
	return tickCmd()
}

func (m *tuiModel) leaveRecord() {
	if m.record.phase == "ready" {
		m.stopCap() // the camera stops when you leave, unless an entry is paused
	}
	m.enterLog()
}

func (m *tuiModel) stopCaptureIfRunning() { m.stopCap() }

func (m *tuiModel) updateRecord(msg tea.Msg) (tea.Model, tea.Cmd) {
	st := &m.record
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	st.lastKey = time.Now()
	if st.resting {
		st.resting = false
		m.startPreview() // any key wakes the camera
		m.status = ""
		return m, nil
	}
	if st.picker.open {
		chosen, done, cancelled, cmd := st.picker.update(key)
		if !done {
			return m, cmd
		}
		if !cancelled {
			m.setMission(chosen)
		}
		return m, nil
	}
	k := key.String()
	if st.confirmDiscard && k != "x" {
		st.confirmDiscard = false
		m.status = "kept"
	}
	switch st.phase {
	case "recording":
		switch k {
		case " ", "p", "esc":
			m.pauseEntry()
		case "tab":
			m.status = "recording: space pauses, enter completes"
		case "enter":
			return m, m.completeEntry()
		case "x":
			return m, m.discardEntry()
		}
	case "paused":
		switch k {
		case " ", "p":
			m.resumeEntry()
		case "enter":
			return m, m.completeEntry()
		case "x":
			return m, m.discardEntry()
		case "esc", "tab":
			m.status = "paused entry kept; space resumes, enter completes, x discards"
		}
	default: // ready
		switch k {
		case " ":
			m.startEntry()
		case "m":
			st.picker.start(strings.TrimSpace(st.mission.Value()), m.missionNamesOnly())
		case "v":
			if m.reflectionOn() {
				m.v.Config.Reflection = "off"
				m.stopCap()
				m.status = "camera off"
			} else {
				m.v.Config.Reflection = "on"
				m.startPreview()
				m.status = "camera on"
			}
			_ = m.v.SaveConfig()
		case "s":
			m.stopCap()
			m.enterSettings()
		case "a":
			m.enterAsk()
		case "tab", "esc":
			m.leaveRecord()

		}
	}
	return m, nil
}

func (m *tuiModel) setMission(name string) {
	m.record.mission.SetValue(name)
	m.v.Config.LastMission = name
	_ = m.v.SaveConfig()
	if name == "" {
		m.status = "free run"
	} else {
		m.status = "mission: " + name
	}
}

func (m *tuiModel) missionNamesOnly() []string {
	var names []string
	for _, e := range m.data.entities {
		if e.Kind == "mission" {
			n := e.ID
			if len(e.Aliases) > 0 {
				n = e.Aliases[0]
			}
			names = append(names, n)
		}
	}
	return names
}

// startPart starts recording one part of the current entry (with the preview if on).
func (m *tuiModel) startPart() bool {
	st := &m.record
	m.stopCap()
	w, h := 0, 0
	if m.reflectionOn() {
		w, h = m.previewSize()
	}
	dir := m.v.Path("inbox", ".parts")
	_ = os.MkdirAll(dir, 0o755)
	out := filepath.Join(dir, fmt.Sprintf("%s.part%02d.mp4", time.Now().Format("2006-01-02T15-04-05"), len(st.parts)+1))
	c, err := openCamera(m.v, captureOptions{Record: true, Out: out, PreviewW: w, PreviewH: h, PreviewFPS: m.previewFPS()})
	if err != nil {
		st.lastErr = err.Error()
		m.status = st.lastErr
		return false
	}
	st.cap = c
	st.parts = append(st.parts, out)
	st.lastErr = ""
	return true
}

func (m *tuiModel) startEntry() {
	st := &m.record
	st.parts, st.recorded, st.completed, st.confirmDiscard = nil, 0, "", false
	m.v.Config.LastMission = strings.TrimSpace(st.mission.Value())
	_ = m.v.SaveConfig()
	if m.startPart() {
		st.phase = "recording"
		m.status = ""
	}
}

func (m *tuiModel) pauseEntry() {
	st := &m.record
	if st.cap != nil {
		st.recorded += st.cap.elapsed()
		_ = st.cap.stop()
		st.cap = nil
	}
	st.phase = "paused"
	m.startPreview()
	m.status = "paused · the same entry continues when you resume"
}

func (m *tuiModel) resumeEntry() {
	if m.totalRecorded() >= m.maxEntry() {
		m.status = "three minutes are up · enter completes this entry"
		return
	}
	if m.startPart() {
		m.record.phase = "recording"
		m.status = ""
	}
}

// maxEntry is how long one entry may run. Three minutes by default: long enough to say
// what happened and how it felt, short enough to do every day.
func (m *tuiModel) maxEntry() time.Duration {
	s := m.v.Config.MaxEntryS
	if s <= 0 {
		s = 180
	}
	return time.Duration(s) * time.Second
}

func (m *tuiModel) totalRecorded() time.Duration {
	st := &m.record
	d := st.recorded
	if st.phase == "recording" && st.cap != nil {
		d += st.cap.elapsed()
	}
	return d
}

// completeEntry joins the parts into one clip in inbox/ and processes it in the background.
func (m *tuiModel) completeEntry() tea.Cmd {
	st := &m.record
	total := m.totalRecorded()
	if st.cap != nil && st.phase == "recording" {
		_ = st.cap.stop()
		st.cap = nil
	}
	parts := st.parts
	st.parts = nil
	st.phase = "ready"
	st.processing = true
	st.procStart = time.Now()
	st.completed = fmt.Sprintf("■ ENTRY COMPLETE  ·  %s", mmss(total.Seconds()))
	m.status = ""
	mission := strings.TrimSpace(st.mission.Value())
	v := m.v
	out := v.Path("inbox", time.Now().Format("2006-01-02T15-04-05")+".mp4")
	// no picture until the entry is processed (pictureMayStart); it comes back in ingestDoneMsg
	return tea.Batch(tickCmd(), func() tea.Msg {
		if err := concatSegments(findTool(v.Config.FFmpegBin), parts, out); err != nil {
			return ingestDoneMsg{err: err}
		}
		ep, err := Ingest(context.Background(), v, IngestOptions{File: out, Mission: mission})
		return ingestDoneMsg{ep: ep, err: err}
	})
}

// discardEntry needs x twice: the first press asks, the second deletes the parts.
func (m *tuiModel) discardEntry() tea.Cmd {
	st := &m.record
	if !st.confirmDiscard {
		st.confirmDiscard = true
		m.status = "discard this entry? press x again; any other key keeps it"
		return nil
	}
	if st.cap != nil && st.phase == "recording" {
		_ = st.cap.stop()
		st.cap = nil
	}
	for _, p := range st.parts {
		_ = os.Remove(p)
	}
	st.parts, st.recorded, st.confirmDiscard = nil, 0, false
	st.phase = "ready"
	m.status = "entry discarded"
	m.startPreview()
	return tickCmd()
}

// recordOverlays is everything written on the record screen, in body coordinates:
// the readout, the mission, the meter, the picker, the completion and processing lines.
func (m *tuiModel) recordOverlays() []overlayText {
	st := &m.record
	now := time.Now()
	// overlays are placed in body coordinates: row 0 is the first line under the frame,
	// col 0 the first column inside it. A margin of one row and three columns all round.
	var rec overlayText
	switch st.phase {
	case "recording":
		dot := "● "
		if (now.UnixMilli()/500)%2 == 1 {
			dot = "  "
		}
		left := m.maxEntry() - m.totalRecorded()
		if left < 0 {
			left = 0
		}
		colour := rgbAmber
		if left <= 30*time.Second {
			colour = rgbEnd // the last half minute leans red
		}
		text := fmt.Sprintf("%sREC  %s   %s left", dot, hhmmss(m.totalRecorded()), mmss(left.Seconds()))
		if st.cap != nil && st.cap.elapsed() == 0 && st.recorded == 0 {
			text = dot + "REC  starting …"
		}
		rec = overlayText{row: 2, col: 3, text: text, rgb: colour}
	case "paused":
		left := m.maxEntry() - m.totalRecorded()
		if left < 0 {
			left = 0
		}
		rec = overlayText{row: 2, col: 3, text: fmt.Sprintf("‖ PAUSED  %s   %s left", hhmmss(m.totalRecorded()), mmss(left.Seconds())), rgb: rgbInk}
	default:
		rec = overlayText{row: 2, col: 3, text: "○ READY  00:00:00", rgb: rgbDim}
	}
	missionText := strings.TrimSpace(st.mission.Value())
	missionRGB := rgbAccent2
	if missionText == "" {
		missionText, missionRGB = "free run", rgbDim
	}
	level := 0.0
	if st.cap != nil {
		level = st.cap.level()
	}
	n := int(level*20 + 0.5)
	bar := strings.Repeat("▮", n) + strings.Repeat("▯", 20-n)
	bottom := m.bodyHeight() - 2 // one empty row between the meter and the frame
	overlays := []overlayText{
		rec,
		{row: 4, col: 3, text: missionText, rgb: missionRGB},
		{row: bottom, col: 3, text: "AUD  ", rgb: rgbDim},
		{row: bottom, col: 8, text: bar, rgb: rgbAmber},
	}
	row := 6
	if st.picker.open {
		lines, chosen := st.picker.rows()
		for i, line := range lines {
			cur, rgb := "  ", rgbMid
			if i == 0 {
				cur, rgb = "", rgbDim
			} else if i == chosen {
				cur, rgb = "▶ ", rgbAmber
			}
			overlays = append(overlays, overlayText{row: row, col: 3, text: cur + line, rgb: rgb})
			row++
		}
		row++
	}
	if st.completed != "" {
		overlays = append(overlays, overlayText{row: row, col: 3, text: st.completed, rgb: rgbOK})
		row += 2 // an empty row before the processing line
	}
	if st.processing {
		stage, pct, done, total := readProgress()
		what := "starting …"
		switch stage {
		case "downloading":
			what = fmt.Sprintf("fetching the speech model, once  %d%%", pct)
		case "starting the local AI":
			what = "starting the local AI"
		case "fetching the local AI, once":
			what = fmt.Sprintf("fetching the local AI, once  %d%%", pct)
		case "transcribing":
			what = fmt.Sprintf("transcribing  %d%%", pct)
		case "extracting":
			if total > 0 {
				what = fmt.Sprintf("extracting  %d of %d", min(done+1, total), total)
			} else {
				what = "extracting"
			}
		}
		overlays = append(overlays,
			overlayText{row: row, col: 3, text: what, rgb: rgbAmber},
			overlayText{row: row, col: 3 + len([]rune(what)) + 3, text: hhmmss(time.Since(st.procStart)), rgb: rgbMid})
		row += 2
	}
	if st.lastErr != "" {
		overlays = append(overlays, overlayText{row: row, col: 3, text: fit(st.lastErr, m.inner()-2), rgb: rgbAmber})
	}
	return overlays
}

func (m *tuiModel) viewRecord() string {
	st := &m.record
	overlays := m.recordOverlays()
	// with a picture: edge to edge, the frame painted over it
	var refl *reflection
	if st.cap != nil {
		refl = st.cap.picture()
	}
	// a host draws the picture behind the text view: only the frame and the text, on a ground
	// the host keeps see-through (docs/HOST.md)
	if m.reflectionOn() && st.cap != nil && refl == nil {
		W, H, all := m.windowOverlays(overlays)
		return windowTintReset + renderTextOver(W, H, all)
	}
	if m.reflectionOn() && refl != nil {
		if f := refl.snapshot(); f != nil {
			W, H, all := m.windowOverlays(overlays)
			if m.clearVideo() {
				// real pixels under the text; the title bar takes the picture's top colour.
				// The picture is sent only when the camera has a new frame or the window
				// changed shape; the terminal keeps showing the last one meanwhile.
				tint := windowTint(edgeColorClear(f, refl.w, refl.h))
				got := refl.frames()
				img := ""
				if m.focused && (got != st.sentFrame || W != st.sentW || H != st.sentH) {
					// a still scene sends nothing: only when the picture has moved since the
					// last frame that was actually sent, or the window changed shape
					if W != st.sentW || H != st.sentH || frameMoved(st.lastSent, f) {
						shown := applyLook(currentLook, currentStrength, f)
						img = kittyFrame(shown, refl.w, refl.h, W, H)
						st.lastSent = append(st.lastSent[:0], f...)
						st.sentW, st.sentH = W, H
					}
					st.sentFrame = got
				}
				return tint + img + renderTextOver(W, H, all)
			}
			tint := windowTint(edgeColor(f, refl.w, refl.h, m.v.Config.Reflection))
			return tint + renderReflection(f, refl.w, refl.h, W, H, m.v.Config.Reflection, all)
		}
	}
	m.fullWindow = false
	// no picture (camera off, or still starting): the mark in the middle
	if (st.phase == "ready" || st.phase == "") && !st.processing && st.completed == "" && st.lastErr == "" && !st.picker.open {
		overlays = append(overlays, wordmarkOverlays(m.inner(), m.bodyHeight())...)
	}
	lines := make([]string, m.bodyHeight())
	for _, o := range overlays {
		if o.row < 0 || o.row >= len(lines) {
			continue
		}
		line := lines[o.row]
		for lipgloss.Width(line) < o.col {
			line += " "
		}
		style := sDim
		switch o.rgb {
		case rgbAmber:
			style = sAmber
		case rgbInk:
			style = sInk
		case rgbMid:
			style = sMid
		case rgbOK:
			style = sOK
		}
		lines[o.row] = line + style.Render(o.text)
	}
	return windowTintReset + kittyClear + strings.Join(lines, "\n")
}

// windowOverlays places the record screen's text for a picture that fills the window, with
// the frame drawn over it, and marks the view as painting the whole window.
func (m *tuiModel) windowOverlays(overlays []overlayText) (W, H int, all []overlayText) {
	W, H = max(20, m.width), max(5, m.height)
	all = make([]overlayText, 0, len(overlays)+H+2)
	for _, o := range overlays {
		all = append(all, overlayText{row: o.row + 1, col: o.col + 2, text: o.text, rgb: o.rgb})
	}
	all = append(all, frameOverlays(m, W, H)...)
	m.fullWindow = true
	return W, H, all
}

// frameOverlays draws the frame as text over the picture: the streak top right, the keys
// bottom left, thin sides.
func frameOverlays(m *tuiModel, W, H int) []overlayText {
	rule := rgbRule
	streak := m.streakText(false)
	top := []rune("╭" + strings.Repeat("─", max(0, W-2)) + "╮")
	tail := []rune(" " + streak + " ─╮")
	if len(tail) < len(top) {
		copy(top[len(top)-len(tail):], tail)
	}
	bottom := []rune("╰" + strings.Repeat("─", max(0, W-2)) + "╯")
	// the lines let the picture through; the words on them take it down a little
	out := []overlayText{
		{row: 0, col: 0, text: string(top), rgb: rule, bg: 0.9},
		{row: H - 1, col: 0, text: string(bottom), rgb: rule, bg: 0.9},
		{row: 0, col: W - 2 - len([]rune(streak)) - 1, text: streak, rgb: rgbAccent2, bg: 0.55},
	}
	// the keys as caps along the bottom line: the key on a dark patch, the label beside it
	col := 3
	for _, h := range m.hints() {
		if h.key != "" {
			cap := " " + h.key + " "
			out = append(out, overlayText{row: H - 1, col: col, text: cap, rgb: rgbInk, bg: 0.18})
			col += len([]rune(cap)) + 1
		}
		out = append(out, overlayText{row: H - 1, col: col, text: h.label, rgb: rgbDim, bg: 0.55})
		col += len([]rune(h.label)) + 3
		if col > W-4 {
			break
		}
	}
	if m.status != "" && col < W-6 {
		out = append(out, overlayText{row: H - 1, col: col, text: fit(m.status, W-4-col), rgb: rgbAmber, bg: 0.55})
	}
	for y := 1; y < H-1; y++ {
		out = append(out, overlayText{row: y, col: 0, text: "│", rgb: rule, bg: 0.9}, overlayText{row: y, col: W - 1, text: "│", rgb: rule, bg: 0.9})
	}
	return out
}

func hhmmss(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, (s%3600)/60, s%60)
}

// wordmark is Poiesis in five rows of blocks.
var wordmark = blockRows("POIESIS")

// blockFont: the letters the mark needs, five rows each, one space between letters.
var blockFont = map[rune][]string{
	'P': {"████ ", "█   █", "████ ", "█    ", "█    "},
	'O': {" ███ ", "█   █", "█   █", "█   █", " ███ "},
	'I': {"███", " █ ", " █ ", " █ ", "███"},
	'E': {"█████", "█    ", "████ ", "█    ", "█████"},
	'S': {" ████", "█    ", " ███ ", "    █", "████ "},
}

func blockRows(word string) []string {
	rows := make([]string, 5)
	for i, ch := range word {
		for r, g := range blockFont[ch] {
			if i > 0 {
				rows[r] += " "
			}
			rows[r] += g
		}
	}
	return rows
}

// wordmarkOverlays centres the mark in a body of w × h cells.
func wordmarkOverlays(w, h int) []overlayText {
	if w < len([]rune(wordmark[0]))+4 || h < 9 {
		return nil
	}
	top := (h - len(wordmark)) / 2
	left := (w - len([]rune(wordmark[0]))) / 2
	var out []overlayText
	for i, row := range wordmark {
		out = append(out, overlayText{row: top + i, col: left, text: row, rgb: rgbAmber, bg: 0.9})
	}
	return out
}

// frameMoved says whether two frames differ enough to be worth drawing: the mean change
// across a sparse sample of pixels, against a small threshold. A person sitting still and
// a static room send nothing; a breath or a hand sends a frame.
func frameMoved(prev, cur []byte) bool {
	if len(prev) != len(cur) || len(cur) == 0 {
		return true
	}
	var sum, n int
	for i := 0; i+2 < len(cur); i += 3 * 41 { // every 41st pixel, spread across rows
		d := int(cur[i]) - int(prev[i])
		if d < 0 {
			d = -d
		}
		sum += d
		n++
	}
	return n == 0 || sum/n >= 3
}

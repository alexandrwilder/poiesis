package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The log page: a search bar, the missions as buttons, and every entry newest first under
// one heading per day. Typing searches. Arrows choose: up and down an entry, left and right
// a mission. Enter opens the entry; what you can do with it lives on the entry itself.

type logRow struct {
	header string   // a day heading, when set
	ep     *Episode // an entry, otherwise
	hit    string   // the line that matched the search
}

type logState struct {
	search  textinput.Model
	filters []string // "all", each mission, "free run"
	filter  int
	rows    []logRow
	cursor  int
	offset  int
	rowsTop int   // body lines above the first row, for the mouse
	chipAt  []int // column where each filter button starts, for the mouse
	chipEnd []int
}

const filterAll = "all"
const filterFree = "free run"

var (
	sChip   = lipgloss.NewStyle().Foreground(cMid).Padding(0, 1)
	sChipOn = lipgloss.NewStyle().Foreground(lipgloss.Color("#141210")).Background(cAmber).Bold(true).Padding(0, 1)
	sBar    = lipgloss.NewStyle().Foreground(cInk)
)

func (m *tuiModel) enterLog() {
	m.scr = screenLog
	m.log.search.Focus()
	m.buildLogRows()
	m.logCursorOnEntry(1)
}

// buildLogRows applies the mission button and the search, then groups by day.
func (m *tuiModel) buildLogRows() {
	st := &m.log
	st.filters = append([]string{filterAll}, missionNames(m.data)...)
	st.filters = append(st.filters, filterFree)
	if st.filter >= len(st.filters) {
		st.filter = 0
	}
	want := st.filters[st.filter]

	q := strings.ToLower(strings.TrimSpace(st.search.Value()))
	hits := map[string]string{}
	if q != "" {
		for _, c := range m.data.searchClaims(q) {
			if _, seen := hits[c.Source.Episode]; !seen {
				hits[c.Source.Episode] = c.Text
			}
		}
	}

	var rows []logRow
	heading := ""
	for i := range m.data.entries {
		e := &m.data.entries[i]
		switch want {
		case filterAll:
		case filterFree:
			if e.Mission != "" {
				continue
			}
		default:
			if humanize(e.Mission) != want {
				continue
			}
		}
		hit := ""
		if q != "" {
			hit = hits[e.ID]
			if hit == "" && !strings.Contains(strings.ToLower(humanize(e.Mission)+" "+e.RecordedAt+" "+m.headline(e.ID)), q) {
				continue
			}
		}
		if h := dayHeading(e); h != heading {
			rows = append(rows, logRow{header: h})
			heading = h
		}
		rows = append(rows, logRow{ep: e, hit: hit})
	}
	st.rows = rows
	if st.cursor >= len(rows) {
		st.cursor = len(rows) - 1
	}
	if st.cursor < 0 {
		st.cursor = 0
	}
}

// dayHeading: "DAY 0042 · THU 03 SEP 2026 · TODAY"
func dayHeading(e *Episode) string {
	t, err := time.Parse(time.RFC3339, e.RecordedAt)
	if err != nil {
		return fmt.Sprintf("DAY %04d", e.Day)
	}
	h := fmt.Sprintf("DAY %04d · %s", e.Day, strings.ToUpper(t.Format("Mon 02 Jan 2006")))
	switch daysAgo(t) {
	case 0:
		h += " · TODAY"
	case 1:
		h += " · YESTERDAY"
	}
	return h
}

func daysAgo(t time.Time) int {
	now := time.Now()
	a := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	b := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())
	return int(a.Sub(b).Hours() / 24)
}

// activityStrip: the last thirty days as marks, entries in amber.
func (m *tuiModel) activityStrip() string {
	have := map[string]bool{}
	for _, e := range m.data.entries {
		if t, err := time.Parse(time.RFC3339, e.RecordedAt); err == nil {
			have[t.Local().Format("2006-01-02")] = true
		}
	}
	now := time.Now()
	var b strings.Builder
	for i := 29; i >= 0; i-- {
		d := now.AddDate(0, 0, -i).Format("2006-01-02")
		if have[d] {
			b.WriteString(sAmber.Render("▪"))
		} else {
			b.WriteString(sRule.Render("·"))
		}
	}
	return b.String() + sDim.Render("  last 30 days")
}

// headline is what the entry was about: its first claim.
func (m *tuiModel) headline(id string) string {
	cs := m.data.byEntry[id]
	if len(cs) == 0 {
		return ""
	}
	return cs[0].Text
}

func missionNames(d *tuiData) []string {
	var out []string
	for _, e := range d.entities {
		if e.Kind == "mission" {
			n := humanize(e.ID)
			if len(e.Aliases) > 0 {
				n = e.Aliases[0]
			}
			out = append(out, n)
		}
	}
	return out
}

// logCursorOnEntry moves the cursor to the next entry row in the given direction.
func (m *tuiModel) logCursorOnEntry(dir int) {
	st := &m.log
	for st.cursor >= 0 && st.cursor < len(st.rows) && st.rows[st.cursor].ep == nil {
		st.cursor += dir
	}
	if st.cursor < 0 {
		st.cursor = 0
		for st.cursor < len(st.rows) && st.rows[st.cursor].ep == nil {
			st.cursor++
		}
	}
	if st.cursor >= len(st.rows) {
		st.cursor = len(st.rows) - 1
		for st.cursor >= 0 && st.rows[st.cursor].ep == nil {
			st.cursor--
		}
	}
}

func (m *tuiModel) logSelected() *Episode {
	st := &m.log
	if st.cursor >= 0 && st.cursor < len(st.rows) {
		return st.rows[st.cursor].ep
	}
	return nil
}

func (m *tuiModel) setFilter(i int) {
	st := &m.log
	if len(st.filters) == 0 {
		return
	}
	st.filter = ((i % len(st.filters)) + len(st.filters)) % len(st.filters)
	m.buildLogRows()
	m.logCursorOnEntry(1)
}

func (m *tuiModel) updateLog(msg tea.Msg) (tea.Model, tea.Cmd) {
	st := &m.log
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "tab":
		return m, m.enterRecord()
	case "esc":
		if st.search.Value() != "" {
			st.search.SetValue("")
			m.buildLogRows()
			m.logCursorOnEntry(1)
			return m, nil
		}
		return m, m.enterRecord()
	case "enter":
		if ep := m.logSelected(); ep != nil {
			for i, e := range m.data.entries {
				if e.ID == ep.ID {
					m.openEntry(i)
					break
				}
			}
		}
		return m, nil
	case "left":
		m.setFilter(st.filter - 1)
		return m, nil
	case "right":
		m.setFilter(st.filter + 1)
		return m, nil
	case "up":
		st.cursor--
		m.logCursorOnEntry(-1)
		return m, nil
	case "down":
		st.cursor++
		m.logCursorOnEntry(1)
		return m, nil
	case "pgup":
		st.cursor -= 10
		m.logCursorOnEntry(-1)
		return m, nil
	case "pgdown":
		st.cursor += 10
		m.logCursorOnEntry(1)
		return m, nil
	}
	// everything else is typing: the search bar is always live
	var cmd tea.Cmd
	st.search, cmd = st.search.Update(msg)
	m.buildLogRows()
	m.logCursorOnEntry(1)
	return m, cmd
}

func (m *tuiModel) viewLog() string {
	st := &m.log
	var b strings.Builder
	inner := m.inner()

	// the search bar on the left, the mission buttons on the right
	bar := sDim.Render("search ") + sBar.Render(st.search.View())
	var chips []string
	st.chipAt, st.chipEnd = nil, nil
	for i, f := range st.filters {
		if i == st.filter {
			chips = append(chips, sChipOn.Render(f))
		} else {
			chips = append(chips, sChip.Render(f))
		}
	}
	chipLine := strings.Join(chips, " ")
	chipW := lipgloss.Width(chipLine)
	barW := inner - chipW - 2
	if barW < 20 {
		barW = 20
	}
	st.search.Width = barW - 10
	bar = sDim.Render("search ") + sBar.Render(st.search.View())
	bar = cut(bar, barW)
	pad := inner - lipgloss.Width(bar) - chipW
	if pad < 1 {
		pad = 1
	}
	x := lipgloss.Width(bar) + pad
	for _, c := range chips {
		st.chipAt = append(st.chipAt, x)
		x += lipgloss.Width(c)
		st.chipEnd = append(st.chipEnd, x)
		x++ // the space between buttons
	}
	b.WriteString(bar + strings.Repeat(" ", pad) + chipLine + "\n")
	b.WriteString(m.activityStrip() + "\n\n")
	st.rowsTop = 3

	if n := m.data.orphans; n > 0 {
		b.WriteString(sAmber.Render(fmt.Sprintf("  %d recording(s) never became entries", n)) + sDim.Render("  ·  in a terminal:  poiesis ingest --orphans") + "\n\n")
		st.rowsTop += 2
	}
	if len(st.rows) == 0 {
		if len(m.data.entries) == 0 {
			b.WriteString(sMid.Render("  nothing here yet.") + "\n")
			b.WriteString(sDim.Render("  press tab, look into the camera, and say what happened today and how it felt."))
		} else {
			b.WriteString(sMid.Render("  nothing matches.") + sDim.Render("  esc clears the search; ← → change the mission."))
		}
		return b.String()
	}
	n := m.bodyHeight() - st.rowsTop
	if n < 3 {
		n = 3
	}
	if st.cursor < st.offset {
		st.offset = st.cursor
	}
	if st.cursor >= st.offset+n {
		st.offset = st.cursor - n + 1
	}
	end := st.offset + n
	if end > len(st.rows) {
		end = len(st.rows)
	}
	for i := st.offset; i < end; i++ {
		r := st.rows[i]
		if r.ep == nil {
			b.WriteString(sAccent2.Render(r.header) + "\n")
			continue
		}
		e := r.ep
		cur, style := "  ", sMid
		if i == st.cursor {
			cur, style = sCursor.Render("▶ "), sInk
		}
		mission := sDim.Render(padRight("free run", 14))
		if e.Mission != "" {
			mission = sAmber.Render(padRight(fit(humanize(e.Mission), 14), 14))
		}
		claims := sDim.Render("        · ")
		switch {
		case e.ClaimCount == 1:
			claims = sMid.Render("   1 claim")
		case e.ClaimCount > 1:
			claims = sMid.Render(fmt.Sprintf("%4d claims", e.ClaimCount))
		}
		text := r.hit
		if text == "" {
			text = m.headline(e.ID)
		}
		if text != "" {
			text = "“" + text + "”"
		}
		line := fmt.Sprintf("%s%s  %s  %s  %s  %s", cur,
			style.Render(clockOf(e.RecordedAt)),
			sDim.Render(mmss(e.DurationS)), claims, mission, sMid.Render(text))
		b.WriteString(cut(line, inner) + "\n")
	}
	return b.String()
}

// clockOf: "16:13"
func clockOf(recordedAt string) string {
	if len(recordedAt) >= 16 {
		return recordedAt[11:16]
	}
	return recordedAt
}

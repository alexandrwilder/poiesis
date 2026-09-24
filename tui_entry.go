package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// The entry screen: the transcript with times, claim tags in the margin, Enter plays the
// second, Tab shows Earlier: older claims about the same people and things.

type entryState struct {
	ep       Episode
	lines    []Line
	tags     []string // claim kind per line ("" when none)
	claimAt  []int    // index into claims for the line's first claim, or -1
	claims   []Claim
	cursor   listState
	earlier  bool
	readback bool            // the read-back: what the log understood from this entry
	heard    []Claim         // every claim of this entry, wrong ones included, in the order said
	wrong    map[string]bool // the claims the person marked wrong
	heardC   listState
	earlierL []Claim
	earlierC listState
	loadErr  string
	mission  missionPicker
	confirmX bool
}

func (m *tuiModel) openEntry(i int) {
	e := m.data.entries[i]
	st := entryState{ep: e, claims: m.data.byEntry[e.ID]}
	lines, err := transcriptLines(m.v, e)
	if err != nil {
		st.loadErr = "transcript not readable: " + err.Error()
	}
	st.lines = lines
	st.tags = make([]string, len(lines))
	st.claimAt = make([]int, len(lines))
	for i := range lines {
		st.claimAt[i] = -1
	}
	for ci, c := range st.claims {
		for li, l := range lines {
			if c.Source.Start >= l.Start-0.05 && c.Source.Start <= l.End+0.05 {
				if st.tags[li] == "" {
					st.tags[li] = c.Kind
					st.claimAt[li] = ci
				}
				break
			}
		}
	}
	st.earlierL = m.data.earlierClaims(e)
	if heard, wrong, err := heardIn(m.v, e.ID); err == nil {
		st.heard, st.wrong = heard, wrong
	} else {
		st.wrong = map[string]bool{}
		m.status = "the read-back could not be read: " + err.Error()
	}
	m.entry = st
	m.scr = screenEntry
}

func (m *tuiModel) updateEntry(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	st := &m.entry
	if st.mission.open {
		chosen, done, cancelled, cmd := st.mission.update(key)
		if !done {
			return m, cmd
		}
		if cancelled {
			return m, nil
		}
		id := st.ep.ID
		if err := SetMission(m.v, id, slugify(chosen)); err != nil {
			m.status = "could not change the mission: " + err.Error()
			return m, nil
		}
		what := chosen
		if what == "" {
			what = "free run"
		}
		return m, reloadCmd(m.v, id+" → "+what, id)
	}
	k := key.String()
	if st.confirmX && k != "x" {
		st.confirmX = false
		m.status = "kept"
	}
	if st.readback {
		switch k {
		case "w":
			return m, m.markHeard()
		case "r", "esc", "q":
			st.readback = false
			return m, nil
		case "enter":
			if st.heardC.cursor >= 0 && st.heardC.cursor < len(st.heard) {
				m.status, _ = playStatus(m.v, st.ep.Media, st.heard[st.heardC.cursor].Source.Start)
			}
			return m, nil
		case "up", "down", "k", "j", "pgup", "pgdown", "home", "end":
			moveCursor(&st.heardC, k, len(st.heard))
			return m, nil
		}
	}
	switch k {
	case "r":
		st.readback, st.earlier = true, false
		return m, nil
	case "esc", "q":
		if st.earlier {
			st.earlier = false
			return m, nil
		}
		m.enterLog()
		return m, nil
	case "tab":
		return m, m.enterRecord()
	case "a":
		m.enterAsk()
		return m, nil
	case " ":
		st.earlier = !st.earlier
		return m, nil
	case "p":
		m.status, _ = playStatus(m.v, st.ep.Media, 0)
		return m, nil
	case "M":
		st.mission.start(humanize(st.ep.Mission), missionNames(m.data))
		return m, nil
	case "x":
		if !st.confirmX {
			st.confirmX = true
			m.status = "move this entry to the vault's trash folder? press x again"
			return m, nil
		}
		st.confirmX = false
		e := st.ep
		if err := TrashEntry(m.v, e); err != nil {
			m.status = "could not move it: " + err.Error()
			return m, nil
		}
		m.scr = screenLog
		return m, reloadCmd(m.v, e.ID+" moved to trash/", "")
	case "enter":
		if st.earlier {
			if len(st.earlierL) > 0 {
				c := st.earlierL[st.earlierC.cursor]
				m.status, _ = playStatus(m.v, m.data.mediaFor(c.Source.Episode), c.Source.Start)
			}
			return m, nil
		}
		if len(st.lines) > 0 {
			l := st.lines[st.cursor.cursor]
			t := l.Start
			if ci := st.claimAt[st.cursor.cursor]; ci >= 0 {
				t = st.claims[ci].Source.Start
			}
			m.status, _ = playStatus(m.v, st.ep.Media, t)
		}
		return m, nil
	case "e":
		if st.earlier || st.cursor.cursor >= len(st.claimAt) {
			return m, nil // no line to follow: the words could not be read
		}
		if ci := st.claimAt[st.cursor.cursor]; ci >= 0 && len(st.claims[ci].About) > 0 {
			m.openEntity(st.claims[ci].About[0])
		}
		return m, nil
	}
	if st.earlier {
		moveCursor(&st.earlierC, key.String(), len(st.earlierL))
	} else {
		moveCursor(&st.cursor, key.String(), len(st.lines))
	}
	return m, nil
}

func playStatus(v *Vault, media string, t float64) (string, error) {
	if media == "" {
		return "recording not found", nil
	}
	s, err := play(v, media, t)
	if err != nil {
		return "cannot play: " + err.Error(), err
	}
	return s, nil
}

func (m *tuiModel) viewEntry() string {
	st := &m.entry
	var b strings.Builder
	mission := "free"
	if st.ep.Mission != "" {
		mission = humanize(st.ep.Mission)
	}
	b.WriteString(sHead.Render(fmt.Sprintf("DAY %04d", st.ep.Day)) + sDim.Render("  ──  ") +
		sMid.Render(strings.Replace(leading(st.ep.RecordedAt, 16), "T", " ", 1)) + sDim.Render("  ──  ") +
		sMid.Render(mmss(st.ep.DurationS)) + sDim.Render("  ──  ") + sMid.Render(mission) + "\n\n")
	if st.loadErr != "" {
		b.WriteString(sAmber.Render(st.loadErr) + "\n")
		return b.String()
	}
	if st.mission.open {
		b.WriteString(st.mission.view())
	}
	h := m.bodyHeight() - 2 - st.mission.lines()
	if st.readback {
		m.viewHeard(&b, h-2)
		return b.String()
	}
	if st.earlier {
		h = h / 2
	}
	start, end := st.cursor.visible(len(st.lines), h)
	for i := start; i < end; i++ {
		l := st.lines[i]
		cur, style := "  ", sMid
		if i == st.cursor.cursor {
			cur, style = sCursor.Render("▶ "), sInk
		}
		tag := strings.Repeat(" ", 11)
		if st.tags[i] != "" {
			tag = sAmber.Render("["+st.tags[i]+"]") + strings.Repeat(" ", max(0, 11-len(st.tags[i])-2))
		}
		text := fit(l.Text, max(10, m.width-24))
		b.WriteString(fmt.Sprintf("%s%s  %s %s\n", cur, sDim.Render(mmss(l.Start)), style.Render(padRight(text, max(10, m.width-24))), tag))
	}
	if st.earlier {
		b.WriteString("\n" + sRule.Render(strings.Repeat("─", max(20, m.width))) + "\n")
		b.WriteString(sDim.Render("EARLIER · older claims about the same people and things") + "\n")
		if len(st.earlierL) == 0 {
			b.WriteString(sMid.Render("  nothing yet: this is the first entry about them") + "\n")
		}
		hs := m.bodyHeight() - h - 5
		s2, e2 := st.earlierC.visible(len(st.earlierL), max(3, hs))
		for i := s2; i < e2; i++ {
			c := st.earlierL[i]
			cur := "  "
			if i == st.earlierC.cursor {
				cur = sCursor.Render("▶ ")
			}
			b.WriteString(cur + claimLine(c, m.width-2, true) + sDim.Render("  "+c.Source.Episode) + "\n")
		}
	}
	return b.String()
}

func padRight(s string, w int) string {
	r := []rune(s)
	if len(r) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(r))
}

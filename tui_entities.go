package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Entities: a list by mentions, then one entity's timeline. Search lives on the log page.

type entityState struct {
	id     string
	claims []Claim
	cursor listState
}

func (m *tuiModel) updateEntities(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "q":
		m.scr = screenLog
	case "enter":
		if len(m.data.entities) > 0 {
			m.openEntity(m.data.entities[m.entities.cursor].ID)
		}
	default:
		moveCursor(&m.entities, key.String(), len(m.data.entities))
	}
	return m, nil
}

func (m *tuiModel) viewEntities() string {
	var b strings.Builder
	b.WriteString(sDim.Render("ENTITIES · by mentions") + "\n\n")
	n := m.bodyHeight() - 2
	start, end := m.entities.visible(len(m.data.entities), n)
	for i := start; i < end; i++ {
		e := m.data.entities[i]
		cur, style := "  ", sMid
		if i == m.entities.cursor {
			cur, style = sCursor.Render("▶ "), sInk
		}
		name := e.ID
		if len(e.Aliases) > 0 {
			name = e.Aliases[0]
		}
		b.WriteString(cut(fmt.Sprintf("%s%s  %s  %s  %s", cur, style.Render(padRight(name, 24)), sDim.Render(padRight(e.Kind, 8)),
			style.Render(fmt.Sprintf("%3d", e.Mentions)), sDim.Render("last "+e.LastSeen)), m.width) + "\n")
	}
	return b.String()
}

func (m *tuiModel) openEntity(id string) {
	m.entity = entityState{id: id, claims: m.data.claimsAbout(id)}
	m.scr = screenEntity
}

func (m *tuiModel) updateEntity(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	st := &m.entity
	switch key.String() {
	case "esc", "q":
		m.scr = screenEntities
	case "enter":
		if len(st.claims) > 0 {
			c := st.claims[st.cursor.cursor]
			m.status, _ = playStatus(m.v, m.data.mediaFor(c.Source.Episode), c.Source.Start)
		}
	default:
		moveCursor(&st.cursor, key.String(), len(st.claims))
	}
	return m, nil
}

func (m *tuiModel) viewEntity() string {
	st := &m.entity
	e := m.v.Entities[st.id]
	name, kind := st.id, "theme"
	if e != nil {
		kind = e.Kind
		if len(e.Aliases) > 0 {
			name = e.Aliases[0]
		}
	}
	var b strings.Builder
	b.WriteString(sHead.Render(name) + sDim.Render(fmt.Sprintf("  ·  %s  ·  %d claims", kind, len(st.claims))) + "\n\n")
	n := m.bodyHeight() - 2
	start, end := st.cursor.visible(len(st.claims), n)
	for i := start; i < end; i++ {
		c := st.claims[i]
		cur := "  "
		if i == st.cursor.cursor {
			cur = sCursor.Render("▶ ")
		}
		b.WriteString(cur + claimLine(c, m.width-2, true) + "\n")
	}
	return b.String()
}

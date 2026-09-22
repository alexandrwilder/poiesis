package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Mouse: the wheel moves the cursor of whatever list is on screen; a click selects a row,
// and a click on the row that is already selected opens or plays it, like Enter.
//
// Rows are mapped from screen lines: the frame takes one line on top, every list screen has a
// title line and a blank line above its rows, so a row's index is y - 4 + the list offset.

const headerRows = 1
const listTop = headerRows + 2

func (m *tuiModel) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return m.sendKey("up")
	case tea.MouseButtonWheelDown:
		return m.sendKey("down")
	case tea.MouseButtonLeft:
		return m.click(msg.X, msg.Y)
	}
	return m, nil
}

// sendKey routes a synthetic key through the normal key handling of the current screen.
func (m *tuiModel) sendKey(name string) (tea.Model, tea.Cmd) {
	var k tea.KeyMsg
	switch name {
	case "up":
		k = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		k = tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		k = tea.KeyMsg{Type: tea.KeyEnter}
	case " ":
		k = tea.KeyMsg{Type: tea.KeySpace}
	default:
		k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
	}
	return m.Update(k)
}

// click selects the row under the pointer; a second click on it opens it.
func (m *tuiModel) click(x, y int) (tea.Model, tea.Cmd) {
	if m.scr == screenRecord {
		return m, nil // recording starts with space, never with a click
	}
	if m.scr == screenLog {
		return m.clickLog(x, y)
	}
	if m.scr == screenSettings {
		return m.clickSettings(y)
	}
	if m.scr == screenAsk {
		if i := m.askRow(y - headerRows); i >= 0 {
			if m.ask.cursor == i {
				return m.sendKey("enter")
			}
			m.ask.cursor = i
		}
		return m, nil
	}
	ls, total := m.currentList()
	if ls == nil {
		return m, nil
	}
	i := y - listTop + ls.offset
	if y < listTop || i < 0 || i >= total {
		return m, nil
	}
	if i == ls.cursor {
		return m.sendKey("enter")
	}
	ls.cursor = i
	return m, nil
}

// currentList is the list the pointer can act on, and how many rows it has.
func (m *tuiModel) currentList() (*listState, int) {
	switch m.scr {
	case screenLog:
		return nil, 0 // the log page has its own rows: handled in clickLog
	case screenEntry:
		if m.entry.earlier {
			return nil, 0 // two lists on one screen: keys only, for now
		}
		return &m.entry.cursor, len(m.entry.lines)
	case screenEntities:
		return &m.entities, len(m.data.entities)
	case screenEntity:
		return &m.entity.cursor, len(m.entity.claims)
	}
	return nil, 0
}

// clickLog selects the entry under the pointer; a click on the selected one opens it.
func (m *tuiModel) clickLog(x, y int) (tea.Model, tea.Cmd) {
	st := &m.log
	if y == headerRows { // the search bar and the mission buttons
		col := x - 2 // the frame's left edge and its space
		for i := range st.chipAt {
			if col >= st.chipAt[i] && col < st.chipEnd[i] {
				m.setFilter(i)
				return m, nil
			}
		}
		return m, nil
	}
	i := y - headerRows - st.rowsTop + st.offset
	if i < 0 || i >= len(st.rows) || st.rows[i].ep == nil {
		return m, nil
	}
	if i == st.cursor {
		return m.sendKey("enter")
	}
	st.cursor = i
	return m, nil
}

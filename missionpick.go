package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// missionPicker chooses a mission for one entry: the existing ones, free run, or a new
// name. Used on the entry screen; the record screen has its own for the next entries.

type missionPicker struct {
	open    bool
	typing  bool
	options []string // missions, then "" for free run
	pick    int
	input   textinput.Model
}

func (p *missionPicker) start(current string, names []string) {
	p.options = append(append([]string{}, names...), "")
	p.pick = len(p.options) - 1
	for i, o := range p.options {
		if o == current {
			p.pick = i
		}
	}
	p.open, p.typing = true, false
	if p.input.Placeholder == "" {
		p.input = newInput("mission name")
	}
}

// update handles one key. done is true when a choice was made (chosen may be "" for free
// run) or the picker was cancelled (then cancelled is true).
func (p *missionPicker) update(msg tea.KeyMsg) (chosen string, done, cancelled bool, cmd tea.Cmd) {
	if p.typing {
		switch msg.String() {
		case "enter":
			p.open, p.typing = false, false
			p.input.Blur()
			return strings.TrimSpace(p.input.Value()), true, false, nil
		case "esc":
			p.open, p.typing = false, false
			p.input.Blur()
			return "", true, true, nil
		}
		p.input, cmd = p.input.Update(msg)
		return "", false, false, cmd
	}
	switch msg.String() {
	case "up", "k":
		if p.pick > 0 {
			p.pick--
		}
	case "down", "j":
		if p.pick < len(p.options) {
			p.pick++
		}
	case "enter":
		if p.pick == len(p.options) {
			p.typing = true
			p.input.SetValue("")
			p.input.Focus()
			return "", false, false, nil
		}
		p.open = false
		return p.options[p.pick], true, false, nil
	case "n":
		p.typing = true
		p.input.SetValue("")
		p.input.Focus()
	case "esc":
		p.open = false
		return "", true, true, nil
	}
	return "", false, false, nil
}

func (p *missionPicker) view() string {
	var b strings.Builder
	if p.typing {
		b.WriteString(sDim.Render("NEW MISSION  ") + p.input.View() + "\n\n")
		return b.String()
	}
	b.WriteString(sDim.Render("MISSION FOR THIS ENTRY   ↑↓ · enter · n new · esc") + "\n")
	for i, o := range p.options {
		label := o
		if o == "" {
			label = "free run"
		}
		cur, style := "  ", sMid
		if i == p.pick {
			cur, style = sCursor.Render("▶ "), sInk
		}
		b.WriteString(cur + style.Render(label) + "\n")
	}
	cur, style := "  ", sMid
	if p.pick == len(p.options) {
		cur, style = sCursor.Render("▶ "), sInk
	}
	b.WriteString(cur + style.Render("new mission …") + "\n\n")
	return b.String()
}

// lines is how many rows the picker takes when open.
func (p *missionPicker) lines() int {
	if !p.open {
		return 0
	}
	if p.typing {
		return 2
	}
	return len(p.options) + 3
}

// rows is the picker as plain text, for screens that draw over a picture: each row and
// whether it is the chosen one. The first row is the title.
func (p *missionPicker) rows() (lines []string, chosen int) {
	if p.typing {
		return []string{"NEW MISSION  " + p.input.Value() + "▏"}, -1
	}
	lines = append(lines, "TAG THE NEXT ENTRIES WITH A MISSION   ↑↓ choose · enter · n new · esc")
	for _, o := range p.options {
		if o == "" {
			o = "free run (no mission)"
		}
		lines = append(lines, o)
	}
	lines = append(lines, "new mission …")
	return lines, p.pick + 1
}

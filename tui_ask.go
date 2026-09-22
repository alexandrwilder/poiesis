package main

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The ask screen: pick a question, or type one; the local AI answers from the log, with
// the moments to play. c copies the question for another AI app.

type askState struct {
	cursor  int // index into the flat list of questions
	own     textinput.Model
	typing  bool
	asking  bool
	asked   string
	answer  string
	problem string
}

type askDoneMsg struct {
	question, answer string
	err              error
}

func (m *tuiModel) askQuestions() []string {
	var out []string
	for _, s := range askSets {
		out = append(out, s.Questions...)
	}
	return out
}

func (m *tuiModel) enterAsk() {
	m.stopCap()
	m.scr = screenAsk
	if m.ask.own.Placeholder == "" {
		m.ask.own = newInput("or type your own question")
	}
}

func (m *tuiModel) updateAsk(msg tea.Msg) (tea.Model, tea.Cmd) {
	st := &m.ask
	switch msg := msg.(type) {
	case askDoneMsg:
		st.asking = false
		st.asked = msg.question
		if msg.err != nil {
			st.problem = msg.err.Error()
			st.answer = ""
		} else {
			st.problem = ""
			st.answer = msg.answer
		}
		return m, nil
	case tea.KeyMsg:
		if st.typing {
			switch msg.String() {
			case "enter":
				q := strings.TrimSpace(st.own.Value())
				st.typing = false
				st.own.Blur()
				if q == "" {
					return m, nil
				}
				return m, m.askCmd(q)
			case "esc":
				st.typing = false
				st.own.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			st.own, cmd = st.own.Update(msg)
			return m, cmd
		}
		qs := m.askQuestions()
		switch msg.String() {
		case "esc", "tab":
			return m, m.enterRecord()
		case "up", "k":
			if st.cursor > 0 {
				st.cursor--
			}
		case "down", "j":
			if st.cursor < len(qs) {
				st.cursor++
			}
		case "enter":
			if st.cursor == len(qs) {
				st.typing = true
				st.own.Focus()
				return m, nil
			}
			return m, m.askCmd(qs[st.cursor])
		case "c":
			q := ""
			if st.cursor < len(qs) {
				q = qs[st.cursor]
			} else {
				q = strings.TrimSpace(st.own.Value())
			}
			if q == "" {
				return m, nil
			}
			if err := copyText(forYourAI(q)); err != nil {
				m.status = "could not copy: " + err.Error()
			} else {
				m.status = "copied, with the line your AI needs first · paste it into Claude, Cursor or any AI app connected to the log"
			}
		}
	}
	return m, nil
}

// askCmd asks in the background; the screen shows "reading the log" meanwhile.
func (m *tuiModel) askCmd(question string) tea.Cmd {
	st := &m.ask
	if st.asking {
		return nil
	}
	st.asking, st.asked, st.answer, st.problem = true, question, "", ""
	v, d := m.v, m.data
	return tea.Batch(tickCmd(), func() tea.Msg {
		ctx := askContext(d, question)
		a, err := askLocal(v, question, ctx)
		return askDoneMsg{question: question, answer: a, err: err}
	})
}

func (m *tuiModel) viewAsk() string {
	st := &m.ask
	var b strings.Builder
	inner := m.inner()
	b.WriteString(sDim.Render("ASK YOUR LOG   the local AI reads your claims and answers with the moments · ") + sAccent2.Render("c") + sDim.Render(" copies a question for any other AI app") + "\n\n")
	i := 0
	for _, set := range askSets {
		b.WriteString(sAccent2.Render(strings.ToUpper(set.Name)) + "\n")
		for _, q := range set.Questions {
			cur, style := "  ", sMid
			if i == st.cursor && !st.typing {
				cur, style = sCursor.Render("▶ "), sInk
			}
			b.WriteString(cut(cur+style.Render(q), inner) + "\n")
			i++
		}
	}
	cur, style := "  ", sMid
	if i == st.cursor || st.typing {
		cur, style = sCursor.Render("▶ "), sInk
	}
	if st.typing {
		b.WriteString(cur + st.own.View() + "\n")
	} else {
		b.WriteString(cur + style.Render("your own question …") + "\n")
	}
	b.WriteString("\n")
	switch {
	case st.asking:
		stage, pct, _, _ := readProgress()
		what := "reading the log …"
		if strings.Contains(stage, "local AI") && pct > 0 {
			what = stage + " " + itoa(pct) + "%"
		} else if stage != "" {
			what = stage + " …"
		}
		b.WriteString(sAmber.Render(what) + "\n")
	case st.problem != "":
		b.WriteString(sAmber.Render(cut("could not answer: "+st.problem, inner)) + "\n")
	case st.answer != "":
		b.WriteString(sDim.Render(cut("Q  "+st.asked, inner)) + "\n")
		for _, line := range wrapText(st.answer, inner-3) {
			b.WriteString("   " + sInk.Render(line) + "\n")
		}
		b.WriteString(sDim.Render("   [entry @seconds] is a moment: open the entry and press enter on that line to play it") + "\n")
	}
	return b.String()
}

func itoa(n int) string { return strconv.Itoa(n) }

// wrapText breaks a paragraph into lines of at most w cells, on spaces.
func wrapText(s string, w int) []string {
	if w < 10 {
		w = 10
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := ""
		for _, word := range words {
			if line == "" {
				line = word
			} else if lipgloss.Width(line)+1+lipgloss.Width(word) <= w {
				line += " " + word
			} else {
				out = append(out, line)
				line = word
			}
		}
		out = append(out, line)
	}
	return out
}

// askRow maps a clicked body line to a question index, or -1.
func (m *tuiModel) askRow(y int) int {
	i := 0
	row := 2 // the title line and a blank
	for _, set := range askSets {
		row++ // the set's name
		for range set.Questions {
			if y == row {
				return i
			}
			row++
			i++
		}
	}
	if y == row {
		return i // "your own question"
	}
	return -1
}

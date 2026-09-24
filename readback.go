package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// The read-back: after an entry, the person sees what the log understood and marks what is
// wrong. A mark is a line appended to the entry's corrections file, never an edit of its claims:
// the claims stay as the extractor wrote them, and a claim marked wrong is left out of
// everything that brings words back (AllClaims), for the person and for any AI.

type correction struct {
	Claim string `json:"claim"`
	Wrong bool   `json:"wrong"`
	At    string `json:"at"`
}

func correctionsPath(v *Vault, epID string) string {
	return v.Path("episodes", epID+".corrections.jsonl")
}

// wrongClaims is the set of an entry's claims the person marked wrong; the latest mark of each
// claim wins, so a mark can be taken back.
func wrongClaims(v *Vault, epID string) (map[string]bool, error) {
	wrong := map[string]bool{}
	f, err := os.Open(correctionsPath(v, epID))
	if os.IsNotExist(err) {
		return wrong, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c correction
		if json.Unmarshal(sc.Bytes(), &c) != nil || c.Claim == "" {
			continue // a line cut short by a crash counts for nothing
		}
		wrong[c.Claim] = c.Wrong
	}
	return wrong, sc.Err()
}

// markClaim appends the person's mark on one claim: wrong, or right again.
func markClaim(v *Vault, epID, claimID string, wrong bool, now time.Time) error {
	line, err := json.Marshal(correction{Claim: claimID, Wrong: wrong, At: now.Format(time.RFC3339)})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(correctionsPath(v, epID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// readClaimsFile reads one entry's claims as the extractor wrote them.
func readClaimsFile(p string) ([]Claim, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Claim
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var c Claim
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		out = append(out, c)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(p), err)
	}
	return out, nil
}

// heardIn is everything the log understood from one entry, in the order it was said, wrong
// claims included, with the person's marks: the read-back's list.
func heardIn(v *Vault, epID string) ([]Claim, map[string]bool, error) {
	claims, err := readClaimsFile(v.Path("episodes", epID+".claims.jsonl"))
	if os.IsNotExist(err) {
		claims, err = nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	sort.SliceStable(claims, func(i, j int) bool { return claims[i].Source.Start < claims[j].Source.Start })
	wrong, err := wrongClaims(v, epID)
	return claims, wrong, err
}

// markHeard is w on the read-back: mark the chosen claim wrong, or right again, then bring the
// pages and the window up to date, so the mark counts at once.
func (m *tuiModel) markHeard() tea.Cmd {
	st := &m.entry
	if st.heardC.cursor >= len(st.heard) {
		return nil
	}
	c := st.heard[st.heardC.cursor]
	wrong := !st.wrong[c.ID]
	if err := markClaim(m.v, st.ep.ID, c.ID, wrong, time.Now()); err != nil {
		m.status = "could not keep the mark: " + err.Error()
		return nil
	}
	st.wrong[c.ID] = wrong
	m.status = "marked wrong: it will not come back"
	if !wrong {
		m.status = "marked right again"
	}
	return tea.Batch(rebuildPagesCmd(m.v.Root, st.ep.ID), reloadCmd(m.v, m.status, ""))
}

// rebuildPagesCmd rewrites the entry's page, the entity pages and the index from the claims,
// in its turn with the entries being processed.
func rebuildPagesCmd(root, epID string) tea.Cmd {
	return func() tea.Msg {
		ingestMu.Lock()
		defer ingestMu.Unlock()
		v, err := OpenVault(root)
		if err == nil {
			err = rewriteEntryPage(v, epID)
		}
		if err == nil {
			err = rebuildEntityPages(v)
		}
		if err == nil {
			err = writeIndex(v)
		}
		if err != nil {
			return pagesRebuiltMsg{err: err}
		}
		return pagesRebuiltMsg{}
	}
}

type pagesRebuiltMsg struct{ err error }

// rewriteEntryPage writes an entry's page again with only the claims the person has not marked
// wrong, so an AI reading the page never meets one. The page is derived: its words and claims
// come from the transcript and the claims file, which stay as they are.
func rewriteEntryPage(v *Vault, epID string) error {
	eps, err := loadEpisodes(v)
	if err != nil {
		return err
	}
	for _, ep := range eps {
		if ep.ID != epID {
			continue
		}
		lines, err := transcriptLines(v, ep)
		if err != nil {
			return fmt.Errorf("the words of %s: %w", epID, err)
		}
		heard, wrong, err := heardIn(v, epID)
		if err != nil {
			return err
		}
		var kept []Claim
		for _, c := range heard {
			if !wrong[c.ID] {
				kept = append(kept, c)
			}
		}
		ep.ClaimCount = len(kept)
		return writeEpisode(v, &ep, lines, kept)
	}
	return fmt.Errorf("no entry %s", epID)
}

// viewHeard is the read-back's list: what the log understood, each with its kind and second.
func (m *tuiModel) viewHeard(b *strings.Builder, height int) {
	st := &m.entry
	b.WriteString(sDim.Render("WHAT I HEARD · ") + sAccent2.Render("w") + sDim.Render(" marks a line wrong, and it never comes back · ") + sAccent2.Render("r") + sDim.Render(" the words") + "\n\n")
	if len(st.heard) == 0 {
		b.WriteString(sMid.Render("  nothing was sorted out of this entry yet") + "\n")
		return
	}
	start, end := st.heardC.visible(len(st.heard), max(3, height))
	for i := start; i < end; i++ {
		c := st.heard[i]
		cur := "  "
		if i == st.heardC.cursor {
			cur = sCursor.Render("▶ ")
		}
		line := claimLine(c, m.width-14, false)
		mark := sDim.Render(mmss(c.Source.Start))
		if st.wrong[c.ID] {
			line = sDim.Render(fit("✗ wrong · "+c.Text, m.width-14))
		}
		b.WriteString(cur + mark + "  " + line + "\n")
	}
}

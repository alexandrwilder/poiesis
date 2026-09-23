package main

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// tuiData is everything the screens read: loaded once, reloaded after an ingest.
type tuiData struct {
	entries  []Episode // newest first
	byID     map[string]Episode
	claims   []Claim            // newest first
	byEntry  map[string][]Claim // claims per episode, by start time
	entities []*Entity          // by mentions
	netInUse string
	orphans  int // recordings in raw/ without an entry
}

type dataReloadedMsg struct {
	data      *tuiData
	status    string
	openEntry string
}

func loadTUIData(v *Vault) (*tuiData, error) {
	d, err := loadTUIDataInner(v)
	if err == nil {
		if o, err := v.Orphans(); err == nil {
			d.orphans = len(o)
		}
	}
	return d, err
}

func loadTUIDataInner(v *Vault) (*tuiData, error) {
	eps, err := loadEpisodes(v)
	if err != nil {
		return nil, err
	}
	claims, err := AllClaims(v)
	if err != nil {
		return nil, err
	}
	d := &tuiData{entries: eps, byID: map[string]Episode{}, claims: claims, byEntry: map[string][]Claim{}}
	for _, e := range eps {
		d.byID[e.ID] = e
	}
	for _, c := range claims {
		d.byEntry[c.Source.Episode] = append(d.byEntry[c.Source.Episode], c)
	}
	for id := range d.byEntry {
		cs := d.byEntry[id]
		sort.Slice(cs, func(i, j int) bool { return cs[i].Source.Start < cs[j].Source.Start })
	}
	if err := v.loadEntities(); err != nil {
		return nil, err
	}
	for _, e := range v.Entities {
		d.entities = append(d.entities, e)
	}
	sort.Slice(d.entities, func(i, j int) bool {
		if d.entities[i].Mentions != d.entities[j].Mentions {
			return d.entities[i].Mentions > d.entities[j].Mentions
		}
		return d.entities[i].ID < d.entities[j].ID
	})
	if v.Config.Extractor == "claude" {
		if os.Getenv("ANTHROPIC_API_KEY") != "" {
			d.netInUse = "" // shown only while a call is in flight
		}
	}
	return d, nil
}

// transcriptLines reads the cached word-timed transcript of an entry and groups it into lines.
func transcriptLines(v *Vault, e Episode) ([]Line, error) {
	b, err := os.ReadFile(v.Path(e.Transcript))
	if err != nil {
		return nil, err
	}
	var wj whisperJSON
	if err := json.Unmarshal(b, &wj); err != nil {
		return nil, err
	}
	var words []Word
	for _, seg := range wj.Transcription {
		if t := strings.TrimSpace(seg.Text); t != "" {
			words = append(words, Word{Text: t, Start: float64(seg.Offsets.From) / 1000, End: float64(seg.Offsets.To) / 1000})
		}
	}
	return Lines(words), nil
}

// earlierClaims: older claims about the entities of this entry, newest first.
func (d *tuiData) earlierClaims(e Episode) []Claim {
	about := map[string]bool{}
	for _, c := range d.byEntry[e.ID] {
		for _, id := range c.About {
			about[id] = true
		}
	}
	var out []Claim
	for _, c := range d.claims {
		if c.Source.Episode == e.ID || c.StatedAt >= e.RecordedAt {
			continue
		}
		for _, id := range c.About {
			if about[id] {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// claimsAbout: every claim about one entity, newest first.
func (d *tuiData) claimsAbout(id string) []Claim {
	var out []Claim
	for _, c := range d.claims {
		if contains(c.About, id) {
			out = append(out, c)
		}
	}
	return out
}

// searchClaims: substring over text and quote, with optional kind: and since: filters.
func (d *tuiData) searchClaims(query string) []Claim {
	var terms []string
	kind, since := "", ""
	for _, tok := range strings.Fields(strings.ToLower(query)) {
		switch {
		case strings.HasPrefix(tok, "kind:"):
			kind = strings.TrimPrefix(tok, "kind:")
		case strings.HasPrefix(tok, "since:"):
			since = strings.TrimPrefix(tok, "since:")
		default:
			terms = append(terms, tok)
		}
	}
	var out []Claim
	for _, c := range d.claims {
		if kind != "" && c.Kind != kind {
			continue
		}
		if n := min(len(since), len(c.StatedAt)); since != "" && c.StatedAt[:n] < since[:n] {
			continue
		}
		hay := strings.ToLower(c.Text + " " + c.Quote + " " + strings.Join(c.About, " "))
		ok := true
		for _, t := range terms {
			if !strings.Contains(hay, t) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, c)
		}
	}
	return out
}

func (d *tuiData) mediaFor(episodeID string) string {
	if e, ok := d.byID[episodeID]; ok {
		return e.Media
	}
	return ""
}

func newInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Prompt = ""
	ti.CharLimit = 120
	return ti
}

// reloadCmd reloads the vault after an ingest and optionally opens the new entry.
func reloadCmd(v *Vault, status, openEntry string) tea.Cmd {
	return func() tea.Msg {
		d, err := loadTUIData(v)
		if err != nil {
			return dataReloadedMsg{status: "reload failed: " + err.Error()}
		}
		return dataReloadedMsg{data: d, status: status, openEntry: openEntry}
	}
}

func claimLine(c Claim, width int, withDate bool) string {
	date := ""
	if withDate {
		date = sDim.Render(c.StatedAt[:10]) + "  "
	}
	tag := sAmber.Render(fmt6(c.Kind))
	return fit(date+tag+"  "+sInk.Render(c.Text)+sDim.Render("  · "+mmss(c.Source.Start)), width)
}

func fmt6(kind string) string {
	if len(kind) > 10 {
		kind = kind[:10]
	}
	return kind + strings.Repeat(" ", 10-len(kind))
}

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type IngestOptions struct {
	File       string
	Mission    string
	Prompt     string // what the entry set out to be about (a record link); saved as prompt
	ClaimsFile string
	KeepInbox  bool
}

// Episode is the frontmatter of episodes/<id>.md.
type Episode struct {
	ID          string   `yaml:"id"`
	Day         int      `yaml:"day"`
	RecordedAt  string   `yaml:"recorded_at"`
	DurationS   float64  `yaml:"duration_s"`
	Media       string   `yaml:"media"`
	SHA256      string   `yaml:"sha256"`
	Transcript  string   `yaml:"transcript"`
	Transcriber string   `yaml:"transcriber"`
	Language    string   `yaml:"language"`
	Detected    string   `yaml:"detected,omitempty"`
	Mission     string   `yaml:"mission"`
	Prompt      string   `yaml:"prompt,omitempty"`
	Vitals      *string  `yaml:"vitals"`
	Entities    []string `yaml:"entities"`
	ClaimCount  int      `yaml:"claims"`
	Extractor   string   `yaml:"extractor"`
}

// ErrNoSpeech means the clip has no words in it: no entry is made, and the video stays.
var ErrNoSpeech = errors.New("nothing was said in this clip, so no entry was made (the video is kept)")

// Ingest runs the whole pipeline for one recording.
// ingestMu lets this program process one entry at a time: two entries recorded close together
// never take the same id or rewrite the same pages at once.
var ingestMu sync.Mutex

func Ingest(ctx context.Context, v *Vault, opts IngestOptions) (*Episode, error) {
	ingestMu.Lock()
	defer ingestMu.Unlock()
	defer setProgress("", 0, 0, 0)
	// what the entries before this one added, read now that it is this one's turn
	if err := v.loadEntities(); err != nil {
		return nil, err
	}
	// 1. register: hash, sidecar, move into raw/
	m, err := Register(v, opts.File, opts.KeepInbox)
	if err != nil {
		return nil, err
	}
	epID, err := v.NextEpisodeID(m.RecordedAt)
	if err != nil {
		return nil, err
	}
	day, err := v.Day(m.RecordedAt)
	if err != nil {
		return nil, err
	}
	// 2. audio
	wav, err := ExtractAudio(v, m)
	if err != nil {
		return nil, err
	}
	// 3. transcript, with the vault's names as a hint
	missionID := ""
	if opts.Mission != "" {
		missionID = slugify(opts.Mission)
		if _, ok := v.Entities[missionID]; !ok {
			v.Entities[missionID] = &Entity{ID: missionID, Kind: "mission", Aliases: []string{opts.Mission}}
		}
	}
	ex, err := NewExtractor(v.Config, opts.ClaimsFile)
	if err != nil {
		return nil, err
	}
	ex.Release(ctx) // a local model left in GPU memory by an earlier run breaks whisper's voice detection
	t, err := Transcribe(v, m, wav, v.NamesHint(opts.Mission))
	if err != nil {
		return nil, err
	}
	if len(t.Words) == 0 {
		_ = v.AppendLog(fmt.Sprintf("## [%s] silent | %s | nothing was said; the video is kept, no entry made",
			time.Now().Format("2006-01-02 15:04"), filepath.Base(m.Path)))
		_ = os.WriteFile(strings.TrimSuffix(v.Path(m.Path), filepath.Ext(m.Path))+".silent", []byte("no words were found in this recording\n"), 0o644)
		return nil, ErrNoSpeech
	}
	lines := Lines(t.Words)
	windows := Windows(lines)
	// 4. extraction, one call per window
	defer ex.Release(ctx)
	now := time.Now()
	var claims []Claim
	var dropped []string
	var newEntities []string
	var stats ExtractStats
	if v.Config.Extractor == "ollama" {
		model := v.Config.Model
		if model == "" {
			model = setupOllamaModel
		}
		if err := ensureOllama(v, model); err != nil {
			return nil, err
		}
	}
	setProgress("extracting", 0, 0, len(windows))
	for wi, w := range windows {
		setProgress("extracting", 0, wi, len(windows))
		res, err := ex.Extract(ctx, ExtractRequest{
			EpisodeID: epID, RecordedAt: m.RecordedAt, Day: day, Mission: opts.Mission,
			Language: t.Language, Detected: t.Detected, Window: w, WindowCount: len(windows), Entities: v.Entities,
		})
		if err != nil {
			return nil, err
		}
		stats.add(res.Stats)
		// every extraction is kept as a training pair (window in, answer out) beside the raw
		// recording, so a small model can later be tuned on real examples without relabelling
		if err := appendTrainingPair(v, m, ex.Name(), w, res); err != nil {
			return nil, err
		}
		for _, rc := range res.Claims {
			c, created, why := validateClaim(v, rc, t, epID, m.RecordedAt, now, ex.Name())
			if why != "" {
				dropped = append(dropped, fmt.Sprintf("%s (%s)", why, truncate(rc.Quote, 60)))
				continue
			}
			newEntities = append(newEntities, created...)
			claims = append(claims, c)
		}
	}
	sort.Slice(claims, func(i, j int) bool { return claims[i].Source.Start < claims[j].Source.Start })
	before := len(claims)
	claims = dedupeClaims(claims)
	if d := before - len(claims); d > 0 {
		dropped = append(dropped, fmt.Sprintf("%d duplicate(s)", d))
	}
	// 5. write the entry page and the claims file
	ep := &Episode{
		ID: epID, Day: day, RecordedAt: m.RecordedAt.Format(time.RFC3339), DurationS: m.DurationS,
		Media: m.Path, SHA256: m.SHA256,
		Transcript:  filepath.ToSlash(strings.TrimSuffix(m.Path, filepath.Ext(m.Path)) + ".words." + t.Model + "." + t.Language + ".json"),
		Transcriber: t.Transcribr + " " + t.Model, Language: t.Language, Detected: t.Detected,
		Mission: missionID, Prompt: opts.Prompt, Entities: claimEntities(claims), ClaimCount: len(claims), Extractor: ex.Name(),
	}
	if err := writeEpisode(v, ep, lines, claims); err != nil {
		return nil, err
	}
	if err := writeClaims(v, epID, claims); err != nil {
		return nil, err
	}
	// 6. rebuild every entity page and the index from all claims in the vault
	if err := rebuildEntityPages(v); err != nil {
		return nil, err
	}
	if err := writeIndex(v); err != nil {
		return nil, err
	}
	// 7. log
	msg := fmt.Sprintf("## [%s] ingest | %s | day %d · %.0f s · %d claims · %d entities", now.Format("2006-01-02 15:04"), epID, day, m.DurationS, len(claims), len(ep.Entities))
	if len(newEntities) > 0 {
		msg += fmt.Sprintf(" (new: %s)", strings.Join(newEntities, ", "))
	}
	msg += fmt.Sprintf(" · %s · transcribed in %.0f s · extraction %s", ex.Name(), t.Seconds, stats)
	if len(dropped) > 0 {
		msg += fmt.Sprintf("\n- dropped %d claim(s) whose quote was not found verbatim: %s", len(dropped), strings.Join(dropped, "; "))
	}
	if err := v.AppendLog(msg); err != nil {
		return nil, err
	}
	return ep, nil
}

// appendTrainingPair writes one JSON line per window to raw/…/<stamp>.extract.<extractor>.jsonl:
// the exact lines the model saw and what it returned, plus the prompt version.
func appendTrainingPair(v *Vault, m *Media, extractor string, w Window, res ExtractResult) error {
	safe := strings.NewReplacer("/", "_", ":", "_", " ", "_").Replace(extractor)
	path := strings.TrimSuffix(v.Path(m.Path), filepath.Ext(m.Path)) + ".extract." + safe + ".jsonl"
	type line struct {
		Window   int           `json:"window"`
		Start    float64       `json:"start"`
		End      float64       `json:"end"`
		Lines    []Line        `json:"lines"`
		Response ExtractResult `json:"response"`
		Prompt   string        `json:"prompt_version"`
		At       string        `json:"at"`
	}
	b, err := json.Marshal(line{Window: w.Index, Start: w.Start, End: w.End, Lines: w.Lines, Response: res, Prompt: promptVersion, At: time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// validateClaim turns a raw model claim into a stored claim, or explains why it is dropped.
// Names the model gave are matched to existing pages (id or alias) in code; unmatched names
// become new entities of the kind the model gave. Returns the ids of entities it created.
func validateClaim(v *Vault, rc rawClaim, t *Transcript, epID string, recorded, now time.Time, extractor string) (Claim, []string, string) {
	var c Claim
	if !contains(claimKinds, rc.Kind) {
		return c, nil, "unknown kind " + rc.Kind
	}
	if strings.TrimSpace(rc.Text) == "" {
		return c, nil, "empty text"
	}
	start, end, ok := LocateQuote(t.Words, rc.Quote)
	if !ok {
		return c, nil, "quote not in transcript"
	}
	var about, created []string
	selfOnly := false
	for _, raw := range rc.About {
		name, kind := splitNameKind(raw)
		if name == "" {
			continue
		}
		if isSelfReference(name) {
			selfOnly = true
			continue
		}
		id := v.ResolveEntity(name)
		if id == "" {
			id = slugify(name)
			if id == "" {
				continue
			}
			v.Entities[id] = &Entity{ID: id, Kind: kind, Aliases: []string{name}}
			created = append(created, id)
		} else if e := v.Entities[id]; e != nil && e.Kind == "theme" && kind != "theme" {
			e.Kind = kind // a page created without a kind learns it the first time the model names one
		}
		if !contains(about, id) {
			about = append(about, id)
		}
		if len(about) == 3 {
			break
		}
	}
	// A claim about the person talking, or a state with no other topic, lives on the "self"
	// page: that page's timeline is the mood and energy series.
	if len(about) == 0 && (selfOnly || rc.Kind == "state") {
		if _, ok := v.Entities["self"]; !ok {
			v.Entities["self"] = &Entity{ID: "self", Kind: "theme", Aliases: []string{"Me"}}
			created = append(created, "self")
		}
		about = []string{"self"}
	}
	if len(about) == 0 {
		return c, nil, "no entity"
	}
	conf := rc.Confidence
	if conf < 0 || conf > 1 {
		conf = 0.5
	}
	c = Claim{
		ID: "clm:" + newULID(now), Kind: rc.Kind, Text: strings.TrimSpace(rc.Text), Quote: strings.TrimSpace(rc.Quote),
		About: about, StatedAt: recorded.Format(time.RFC3339),
		Source: Source{Episode: epID, Start: round1(start), End: round1(end)}, Confidence: conf,
		ExtractedAt: now.UTC().Format(time.RFC3339), Extractor: extractor,
	}
	if (rc.Kind == "intention" || rc.Kind == "prediction") && rc.Due != "" {
		if _, err := time.Parse("2006-01-02", rc.Due); err == nil {
			d := rc.Due
			c.Due = &d
		}
	}
	return c, created, ""
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

// splitNameKind turns "Sara (person)" into ("Sara", "person"); "the oven (thing)" into
// ("oven", "theme") since "thing" is not a page kind; a bare name gets kind "theme".
func splitNameKind(raw string) (name, kind string) {
	name, kind = strings.TrimSpace(raw), "theme"
	if i := strings.LastIndex(name, "("); i > 0 && strings.HasSuffix(name, ")") {
		k := strings.ToLower(strings.TrimSpace(name[i+1 : len(name)-1]))
		name = strings.TrimSpace(name[:i])
		if contains(entityKinds, k) {
			kind = k
		}
	}
	for _, p := range []string{"the ", "The ", "a ", "an ", "my ", "our "} {
		if strings.HasPrefix(name, p) && len(name) > len(p) {
			name = name[len(p):]
			break
		}
	}
	return name, kind
}

// isSelfReference catches the model naming the person talking as a topic.
func isSelfReference(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "speaker", "the speaker", "i", "me", "myself", "self", "jag", "mig", "narrator", "user", "author":
		return true
	}
	return false
}

// dedupeClaims drops repeats of the same quote or the same sentence within one entry.
func dedupeClaims(claims []Claim) []Claim {
	seen := map[string]bool{}
	var out []Claim
	for _, c := range claims {
		kq, kt := "q:"+normalizeText(c.Quote), "t:"+normalizeText(c.Text)
		if seen[kq] || seen[kt] {
			continue
		}
		seen[kq], seen[kt] = true, true
		out = append(out, c)
	}
	return out
}

func claimEntities(claims []Claim) []string {
	seen := map[string]bool{}
	var ids []string
	for _, c := range claims {
		for _, id := range c.About {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	sort.Strings(ids)
	return ids
}

func frontmatter(v any) (string, error) {
	b, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	return "---\n" + string(b) + "---\n", nil
}

func writeEpisode(v *Vault, ep *Episode, lines []Line, claims []Claim) error {
	fm, err := frontmatter(ep)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString(fm)
	fmt.Fprintf(&b, "\n# %s · day %d\n\n", ep.ID, ep.Day)
	fmt.Fprintf(&b, "%s · %s · %s", leading(ep.RecordedAt, 16), mmss(ep.DurationS), ep.Transcriber)
	if ep.Mission != "" {
		fmt.Fprintf(&b, " · mission [[%s]]", ep.Mission)
	}
	b.WriteString("\n\n## Claims\n\n")
	if len(claims) == 0 {
		b.WriteString("_none_\n")
	}
	for _, c := range claims {
		links := make([]string, len(c.About))
		for i, id := range c.About {
			links[i] = "[[" + id + "]]"
		}
		fmt.Fprintf(&b, "- **%s** [%s](poiesis://%s?t=%.1f) %s — \"%s\" · %s\n", c.Kind, mmss(c.Source.Start), ep.ID, c.Source.Start, c.Text, c.Quote, strings.Join(links, " "))
	}
	b.WriteString("\n## Transcript\n\n")
	for _, l := range lines {
		fmt.Fprintf(&b, "[%s] %s\n", mmss(l.Start), l.Text)
	}
	return os.WriteFile(v.Path("episodes", ep.ID+".md"), []byte(b.String()), 0o644)
}

func writeClaims(v *Vault, epID string, claims []Claim) error {
	f, err := os.OpenFile(v.Path("episodes", epID+".claims.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, c := range claims {
		line, err := json.Marshal(c)
		if err != nil {
			return err
		}
		w.Write(line)
		w.WriteByte('\n')
	}
	return w.Flush()
}

// AllClaims reads every claims file in the vault.
func AllClaims(v *Vault) ([]Claim, error) {
	matches, err := filepath.Glob(v.Path("episodes", "*.claims.jsonl"))
	if err != nil {
		return nil, err
	}
	var all []Claim
	for _, p := range matches {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			if strings.TrimSpace(sc.Text()) == "" {
				continue
			}
			var c Claim
			if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
				f.Close()
				return nil, fmt.Errorf("%s: %w", filepath.Base(p), err)
			}
			all = append(all, c)
		}
		f.Close()
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].StatedAt != all[j].StatedAt {
			return all[i].StatedAt > all[j].StatedAt // newest first
		}
		return all[i].Source.Start < all[j].Source.Start
	})
	return all, nil
}

// loadEpisodes reads the frontmatter of every entry page, newest first.
func loadEpisodes(v *Vault) ([]Episode, error) {
	ids, err := v.EpisodeIDs()
	if err != nil {
		return nil, err
	}
	var eps []Episode
	for _, id := range ids {
		b, err := os.ReadFile(v.Path("episodes", id+".md"))
		if err != nil {
			return nil, err
		}
		fm, _ := splitFrontmatter(string(b))
		var e Episode
		if err := yaml.Unmarshal([]byte(fm), &e); err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		eps = append(eps, e)
	}
	sort.Slice(eps, func(i, j int) bool { return eps[i].ID > eps[j].ID })
	return eps, nil
}

// rebuildEntityPages writes entities/<id>.md for every entity, mechanically, from all claims.
// A mission page also lists the entries recorded under it.
func rebuildEntityPages(v *Vault) error {
	all, err := AllClaims(v)
	if err != nil {
		return err
	}
	eps, err := loadEpisodes(v)
	if err != nil {
		return err
	}
	byMission := map[string][]Episode{}
	for _, e := range eps {
		if e.Mission != "" {
			byMission[e.Mission] = append(byMission[e.Mission], e)
		}
	}
	byEntity := map[string][]Claim{}
	for _, c := range all {
		for _, id := range c.About {
			byEntity[id] = append(byEntity[id], c)
		}
	}
	for id, e := range v.Entities {
		cs := byEntity[id]
		missionEps := byMission[id]
		e.Mentions = len(cs)
		if len(cs) > 0 {
			e.FirstSeen = leading(cs[len(cs)-1].StatedAt, 10)
			e.LastSeen = leading(cs[0].StatedAt, 10)
		}
		if e.Kind == "mission" && len(missionEps) > 0 {
			e.Mentions = len(missionEps)
			e.FirstSeen = leading(missionEps[len(missionEps)-1].RecordedAt, 10)
			e.LastSeen = leading(missionEps[0].RecordedAt, 10)
		}
		if len(e.Aliases) == 0 {
			e.Aliases = []string{humanize(id)}
		}
		fm, err := frontmatter(e)
		if err != nil {
			return err
		}
		var b strings.Builder
		b.WriteString(fm)
		fmt.Fprintf(&b, "\n# %s\n\n%s", e.Aliases[0], e.Kind)
		if e.Kind == "mission" {
			fmt.Fprintf(&b, " · %d entries", len(missionEps))
		} else {
			fmt.Fprintf(&b, " · %d claim(s)", e.Mentions)
		}
		if e.FirstSeen != "" {
			fmt.Fprintf(&b, " · %s → %s", e.FirstSeen, e.LastSeen)
		}
		if e.Kind == "mission" {
			b.WriteString("\n\n## Entries\n\n")
			if len(missionEps) == 0 {
				b.WriteString("_no entries yet_\n")
			}
			for _, ep := range missionEps {
				fmt.Fprintf(&b, "- [[%s]] · day %d · %s · %s · %d claims\n", ep.ID, ep.Day, strings.Replace(leading(ep.RecordedAt, 16), "T", " ", 1), mmss(ep.DurationS), ep.ClaimCount)
			}
		}
		b.WriteString("\n## Timeline\n\n")
		if len(cs) == 0 {
			b.WriteString("_no claims yet_\n")
		}
		for _, c := range cs {
			super := ""
			if c.ValidTo != nil {
				super = " ~~superseded~~"
			}
			fmt.Fprintf(&b, "- %s · **%s** · %s — \"%s\" · [[%s]] [%s](poiesis://%s?t=%.1f)%s\n",
				leading(c.StatedAt, 10), c.Kind, c.Text, c.Quote, c.Source.Episode, mmss(c.Source.Start), c.Source.Episode, c.Source.Start, super)
		}
		// related: entities that share claims with this one
		related := map[string]int{}
		for _, c := range cs {
			for _, other := range c.About {
				if other != id {
					related[other]++
				}
			}
		}
		if len(related) > 0 {
			b.WriteString("\n## Related\n\n")
			type kv struct {
				id string
				n  int
			}
			var rs []kv
			for k, n := range related {
				rs = append(rs, kv{k, n})
			}
			sort.Slice(rs, func(i, j int) bool { return rs[i].n > rs[j].n || (rs[i].n == rs[j].n && rs[i].id < rs[j].id) })
			for _, r := range rs {
				fmt.Fprintf(&b, "- [[%s]] · %d\n", r.id, r.n)
			}
		}
		if err := os.WriteFile(v.Path("entities", id+".md"), []byte(b.String()), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// writeIndex rewrites _index.md: counts, recent entries, entities by mentions, missions.
func writeIndex(v *Vault) error {
	all, err := AllClaims(v)
	if err != nil {
		return err
	}
	eps, err := loadEpisodes(v)
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntype: index\nupdated: %s\nentries: %d\nclaims: %d\nentities: %d\n---\n\n# Index\n\n", time.Now().Format(time.RFC3339), len(eps), len(all), len(v.Entities))
	b.WriteString("Rewritten by the app after every ingest. Read SCHEMA.md once, this file first, then drill: an entity page, then an episode page around the second you need.\n\n")
	b.WriteString("## Entries, newest first\n\n")
	for i, e := range eps {
		if i == 40 {
			fmt.Fprintf(&b, "- … and %d more\n", len(eps)-40)
			break
		}
		mission := "free"
		if e.Mission != "" {
			mission = "[[" + e.Mission + "]]"
		}
		fmt.Fprintf(&b, "- [[%s]] · day %d · %s · %s · %d claims · %s\n", e.ID, e.Day, strings.Replace(leading(e.RecordedAt, 16), "T", " ", 1), mmss(e.DurationS), e.ClaimCount, mission)
	}
	b.WriteString("\n## Entities by mentions\n\n")
	ents := make([]*Entity, 0, len(v.Entities))
	for _, e := range v.Entities {
		ents = append(ents, e)
	}
	sort.Slice(ents, func(i, j int) bool {
		if ents[i].Mentions != ents[j].Mentions {
			return ents[i].Mentions > ents[j].Mentions
		}
		return ents[i].ID < ents[j].ID
	})
	for _, e := range ents {
		fmt.Fprintf(&b, "- [[%s]] · %s · %d · last %s\n", e.ID, e.Kind, e.Mentions, e.LastSeen)
	}
	b.WriteString("\n## Open loops\n\n")
	open := 0
	for _, c := range all {
		if (c.Kind == "question" || c.Kind == "intention" || c.Kind == "prediction") && c.ValidTo == nil && c.Outcome == nil {
			fmt.Fprintf(&b, "- %s · **%s** · %s · [[%s]]\n", leading(c.StatedAt, 10), c.Kind, c.Text, c.Source.Episode)
			open++
			if open == 30 {
				break
			}
		}
	}
	if open == 0 {
		b.WriteString("_none_\n")
	}
	return os.WriteFile(v.Path("_index.md"), []byte(b.String()), 0o644)
}

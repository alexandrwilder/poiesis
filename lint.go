package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Lint checks the vault's promises: every claim's quote is in its transcript, every entity
// it points at has a page, every supersedes target exists, every recording is on disk.
func Lint(v *Vault) ([]string, error) {
	var problems []string
	ids, err := v.EpisodeIDs()
	if err != nil {
		return nil, err
	}
	all, err := AllClaims(v)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, c := range all {
		known[c.ID] = true
	}
	transcripts := map[string][]Word{}
	for _, id := range ids {
		b, err := os.ReadFile(v.Path("episodes", id+".md"))
		if err != nil {
			return nil, err
		}
		fm, _ := splitFrontmatter(string(b))
		var ep Episode
		if err := yaml.Unmarshal([]byte(fm), &ep); err != nil {
			problems = append(problems, fmt.Sprintf("%s: bad frontmatter: %v", id, err))
			continue
		}
		if _, err := os.Stat(v.Path(ep.Media)); err != nil {
			problems = append(problems, fmt.Sprintf("%s: recording missing: %s", id, ep.Media))
		}
		wb, err := os.ReadFile(v.Path(ep.Transcript))
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: transcript missing: %s", id, ep.Transcript))
			continue
		}
		var wj whisperJSON
		if err := json.Unmarshal(wb, &wj); err != nil {
			problems = append(problems, fmt.Sprintf("%s: transcript unreadable: %v", id, err))
			continue
		}
		var words []Word
		for _, seg := range wj.Transcription {
			if t := strings.TrimSpace(seg.Text); t != "" {
				words = append(words, Word{Text: t, Start: float64(seg.Offsets.From) / 1000, End: float64(seg.Offsets.To) / 1000})
			}
		}
		transcripts[id] = words
	}
	for _, c := range all {
		words, ok := transcripts[c.Source.Episode]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: claim %s points at unknown episode %s", c.Source.Episode, c.ID, c.Source.Episode))
			continue
		}
		if _, _, found := LocateQuote(words, c.Quote); !found {
			problems = append(problems, fmt.Sprintf("%s: claim %s quote not in transcript: %q", c.Source.Episode, c.ID, truncate(c.Quote, 60)))
		}
		if !contains(claimKinds, c.Kind) {
			problems = append(problems, fmt.Sprintf("%s: claim %s has unknown kind %q", c.Source.Episode, c.ID, c.Kind))
		}
		if len(c.About) == 0 {
			problems = append(problems, fmt.Sprintf("%s: claim %s is about nothing", c.Source.Episode, c.ID))
		}
		for _, id := range c.About {
			if _, err := os.Stat(v.Path("entities", id+".md")); err != nil {
				problems = append(problems, fmt.Sprintf("%s: claim %s points at entity without a page: %s", c.Source.Episode, c.ID, id))
			}
		}
		if c.Supersedes != nil && !known[*c.Supersedes] {
			problems = append(problems, fmt.Sprintf("%s: claim %s supersedes unknown claim %s", c.Source.Episode, c.ID, *c.Supersedes))
		}
	}
	return problems, nil
}

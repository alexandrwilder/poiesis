package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Handling entries from the log page: change the mission, or move an entry to the vault's
// trash folder. Nothing is ever deleted outright: the video and the page are moved, so
// "I want my data" also means "I can undo my own tidying".

// SetMission rewrites one entry's mission and rebuilds the pages that mention it.
func SetMission(v *Vault, id, mission string) error {
	p := v.Path("episodes", id+".md")
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	done := false
	for i, l := range lines {
		if strings.HasPrefix(l, "mission:") {
			lines[i] = "mission: " + mission
			done = true
			break
		}
		if i > 0 && l == "---" {
			break // end of the frontmatter
		}
	}
	if !done {
		return fmt.Errorf("%s has no mission line", id)
	}
	if err := writeFileAtomic(p, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return err
	}
	if err := rebuildEntityPages(v); err != nil {
		return err
	}
	if err := writeIndex(v); err != nil {
		return err
	}
	what := mission
	if what == "" {
		what = "free mode"
	}
	return v.AppendLog(fmt.Sprintf("## [%s] mission | %s | %s", time.Now().Format("2006-01-02 15:04"), id, what))
}

// TrashEntry moves the entry page, its video and its claims out of the way.
func TrashEntry(v *Vault, ep Episode) error {
	stamp := time.Now().Format("20060102-150405")
	dir := v.Path("trash", stamp+"_"+ep.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// 1. the page
	if err := moveIfExists(v.Path("episodes", ep.ID+".md"), filepath.Join(dir, ep.ID+".md")); err != nil {
		return err
	}
	// 2. the video, its sidecar and the transcript
	for _, rel := range []string{ep.Media, ep.Media + ".json", strings.TrimSuffix(ep.Media, filepath.Ext(ep.Media)) + ".json", ep.Transcript} {
		if rel == "" {
			continue
		}
		if err := moveIfExists(v.Path(rel), filepath.Join(dir, filepath.Base(rel))); err != nil {
			return err
		}
	}
	// 3. the claims: the entry's own file, moved along with it
	if err := moveIfExists(v.Path("episodes", ep.ID+".claims.jsonl"), filepath.Join(dir, ep.ID+".claims.jsonl")); err != nil {
		return err
	}
	if err := rebuildEntityPages(v); err != nil {
		return err
	}
	if err := writeIndex(v); err != nil {
		return err
	}
	return v.AppendLog(fmt.Sprintf("## [%s] trash | %s | moved to trash/%s", time.Now().Format("2006-01-02 15:04"), ep.ID, filepath.Base(dir)))
}

func moveIfExists(from, to string) error {
	if _, err := os.Stat(from); err != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	// different filesystem: copy, then remove
	if err := copyFile(from, to); err != nil {
		return err
	}
	return os.Remove(from)
}

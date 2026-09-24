package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Nothing recorded is ever lost. An entry cut short by a quit or a crash leaves its parts in
// inbox/.parts; a clip can wait in the inbox; a recording can sit in raw/ without its entry.
// The next window to open takes all of it up and processes it, one entry at a time.

// waitingRecording is one recording an earlier session left to process. inRaw marks one that
// is already in raw/.
type waitingRecording struct {
	file  string
	inRaw bool
}

// waitingRecordings is everything an earlier session left to process: the parts of an
// interrupted entry (joined first), clips in the inbox, and recordings in raw/ without an entry.
func (v *Vault) waitingRecordings() ([]waitingRecording, error) {
	if _, err := recoverParts(v); err != nil {
		return nil, fmt.Errorf("the parts of an unfinished entry: %w", err)
	}
	var out []waitingRecording
	inbox, err := v.InboxFiles()
	if err != nil {
		return nil, err
	}
	for _, f := range inbox {
		out = append(out, waitingRecording{file: f})
	}
	orphans, err := v.Orphans()
	if err != nil {
		return nil, err
	}
	for _, f := range orphans {
		out = append(out, waitingRecording{file: f, inRaw: true})
	}
	return out, nil
}

// recoverParts joins what an interrupted session left in inbox/.parts into clips in inbox/,
// one per entry (an entry's parts start at part01), so they are processed like any other clip.
// A part no program can read is kept in inbox/.parts/unreadable/, never deleted.
func recoverParts(v *Vault) ([]string, error) {
	dir := v.Path("inbox", ".parts")
	parts, err := filepath.Glob(filepath.Join(dir, "*.part*.mp4"))
	if err != nil || len(parts) == 0 {
		return nil, err
	}
	sort.Strings(parts) // named by the time each part started: the order they were recorded
	var groups [][]string
	for _, p := range parts {
		if len(groups) == 0 || strings.HasSuffix(p, ".part01.mp4") {
			groups = append(groups, nil)
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], p)
	}
	ffprobe, ffmpeg := findTool(v.Config.FFprobeBin), findTool(v.Config.FFmpegBin)
	var clips []string
	for _, g := range groups {
		readable, _, err := readableParts(ffprobe, g)
		if err != nil {
			return clips, err
		}
		if len(readable) == 0 {
			continue
		}
		out := v.Path("inbox", strings.SplitN(filepath.Base(readable[0]), ".part", 2)[0]+".mp4")
		if err := concatSegments(ffmpeg, readable, out); err != nil {
			return clips, err
		}
		clips = append(clips, out)
	}
	return clips, nil
}

// readableParts keeps the parts a program can read, in order, and says how many it kept aside:
// the others are moved to unreadable/ beside them, never deleted. One broken part never costs
// the whole entry.
func readableParts(ffprobe string, parts []string) (readable []string, keptAside int, err error) {
	for _, p := range parts {
		if pr, err := probe(ffprobe, p); err == nil {
			if d, _ := strconv.ParseFloat(pr.Format.Duration, 64); d > 0 {
				readable = append(readable, p)
				continue
			}
		}
		dir := filepath.Join(filepath.Dir(p), "unreadable")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return readable, keptAside, err
		}
		if err := os.Rename(p, filepath.Join(dir, filepath.Base(p))); err != nil {
			return readable, keptAside, err
		}
		keptAside++
	}
	return readable, keptAside, nil
}

// takeRecoveryLock lets one Poiesis at a time take up what an earlier session left, so two
// windows started in the same moment never both do. A lock older than ten minutes was left by
// a crash and is taken over.
func takeRecoveryLock() (release func(), ok bool) {
	path := filepath.Join(appStateDir(), "recover.lock")
	_ = os.MkdirAll(appStateDir(), 0o755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		fi, serr := os.Stat(path)
		if serr != nil || time.Since(fi.ModTime()) < 10*time.Minute {
			return nil, false
		}
		_ = os.Remove(path)
		if f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644); err != nil {
			return nil, false
		}
	}
	_ = f.Close()
	return func() { _ = os.Remove(path) }, true
}

// processWaiting takes up what an earlier session left behind (waitingRecordings) when the
// window opens. Each is processed on a vault of its own, opened inside the processing lock,
// one at a time.
func (m *tuiModel) processWaiting() tea.Cmd {
	release, ok := takeRecoveryLock()
	if !ok {
		return nil // another Poiesis is taking it up right now
	}
	items, err := m.v.waitingRecordings()
	release()
	if err != nil {
		m.status = "could not look for unfinished entries: " + err.Error()
		return nil
	}
	if len(items) == 0 {
		return nil
	}
	root := m.v.Root
	m.record.processing = true
	m.record.pending += len(items)
	m.record.procStart = time.Now()
	m.status = fmt.Sprintf("%d recording(s) from before were waiting · processing them now", len(items))
	var cmds []tea.Cmd
	for _, it := range items {
		cmds = append(cmds, func() tea.Msg {
			ep, err := IngestInto(context.Background(), root, IngestOptions{File: it.file, KeepInbox: it.inRaw})
			return ingestDoneMsg{ep: ep, err: err}
		})
	}
	return tea.Batch(cmds...)
}

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
)

// Hearing the language before reading. With the language setting on auto, the general model
// listens to the start, the middle and the end of a recording. Only one switch exists: a
// recording that is clearly Swedish in all three places is read by the Swedish model, when
// it is on this machine. Everything else, a mix of languages included, is read by the
// general model with its own detection, which handles mixed speech best (day 0). The first
// real entry opened in Swedish and went on in English: its start alone said Swedish with
// p = 0.99, so one place is never enough.

// langGuess is what the general model hears in one stretch of a recording.
type langGuess struct {
	Lang string
	P    float64
}

const clearP = 0.8 // below this, a stretch does not count as clear

var detectedLine = regexp.MustCompile(`auto-detected language: ([a-z]+) \(p = ([0-9.]+)\)`)

// routeLanguage returns "sv" when the Swedish model is here and the recording is clearly
// Swedish throughout, and "" otherwise. Without the Swedish model it costs nothing.
func routeLanguage(v *Vault, dirs []string, wav string, dur float64) string {
	kb := findInDirs(dirs, v.Config.KBModel)
	general := findInDirs(dirs, v.Config.TurboModel)
	vad := findInDirs(dirs, v.Config.VADModel)
	if kb == "" || general == "" || vad == "" {
		return ""
	}
	setProgress("transcribing", 0, 0, 0)
	if clearLanguage(detectLanguages(v, wav, general, vad, dur)) == "sv" {
		return "sv"
	}
	return ""
}

// detectLanguages asks the general model which language is spoken in half-minute stretches
// at the places listenAt picks, speech only. A stretch without speech gives no guess.
func detectLanguages(v *Vault, wav, model, vad string, dur float64) []langGuess {
	var out []langGuess
	for i, at := range listenAt(dur) {
		part := filepath.Join(os.TempDir(), fmt.Sprintf("poiesis-lang-%d-%d.wav", os.Getpid(), i))
		cut := exec.Command(findTool(v.Config.FFmpegBin), "-v", "error", "-y",
			"-ss", fmt.Sprintf("%.1f", at), "-t", "30", "-i", wav, "-c", "copy", part)
		if err := cut.Run(); err != nil {
			continue
		}
		b, _ := whisperCmd(v, []string{"-m", model, "-f", part, "-l", "auto", "-dl",
			"--vad", "-vm", vad, "-t", fmt.Sprint(v.Config.Threads)}).CombinedOutput()
		_ = os.Remove(part)
		if g, ok := parseDetected(string(b)); ok {
			out = append(out, g)
		}
	}
	return out
}

// listenAt says where to listen, in seconds: the start, the middle and the last half
// minute, fewer places for a short recording.
func listenAt(dur float64) []float64 {
	switch {
	case dur <= 40:
		return []float64{0}
	case dur <= 75:
		return []float64{0, dur - 30}
	}
	return []float64{0, dur/2 - 15, dur - 30}
}

// parseDetected reads whisper-cli's "auto-detected language: sv (p = 0.99)" line.
func parseDetected(s string) (langGuess, bool) {
	m := detectedLine.FindStringSubmatch(s)
	if m == nil {
		return langGuess{}, false
	}
	p, err := strconv.ParseFloat(m[2], 64)
	if err != nil {
		return langGuess{}, false
	}
	return langGuess{Lang: m[1], P: p}, true
}

// clearLanguage is the language every stretch agrees on with at least clearP certainty,
// or "" when they disagree, one is unsure, or nothing was heard.
func clearLanguage(gs []langGuess) string {
	if len(gs) == 0 {
		return ""
	}
	for _, g := range gs {
		if g.Lang != gs[0].Lang || g.P < clearP {
			return ""
		}
	}
	return gs[0].Lang
}

// findInDirs returns the first non-empty file with this name in the folders, or "".
func findInDirs(dirs []string, name string) string {
	for _, d := range dirs {
		p := filepath.Join(d, name)
		if st, err := os.Stat(p); err == nil && st.Size() > 0 {
			return p
		}
	}
	return ""
}

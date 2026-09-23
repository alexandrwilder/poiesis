package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// Word is one transcribed word with its time in seconds.
type Word struct {
	Text  string  `json:"w"`
	Start float64 `json:"s"`
	End   float64 `json:"e"`
}

// Transcript is what whisper produced for one recording.
type Transcript struct {
	Words      []Word  `json:"words"`
	Model      string  `json:"model"`       // e.g. large-v3-turbo-q5_0
	Language   string  `json:"language"`    // setting used: auto, sv, en
	Detected   string  `json:"detected"`    // language whisper detected, if auto
	Transcribr string  `json:"transcriber"` // e.g. whisper.cpp
	Seconds    float64 `json:"seconds"`     // wall time of the run
}

// whisperJSON is the shape of whisper-cli -ojf output (only the parts used).
type whisperJSON struct {
	Result struct {
		Language string `json:"language"`
	} `json:"result"`
	Transcription []struct {
		Text    string `json:"text"`
		Offsets struct {
			From int64 `json:"from"`
			To   int64 `json:"to"`
		} `json:"offsets"`
	} `json:"transcription"`
}

// Transcribe runs whisper-cli with the language rule from day 0:
// auto/en -> large-v3-turbo with the setting; sv -> KB-Whisper forced Swedish. On auto, a
// recording that is clearly Swedish throughout is read as sv (language.go).
// The names hint is the vault's entity vocabulary. Output is cached next to the raw file.
func Transcribe(v *Vault, m *Media, wav string, names string) (*Transcript, error) {
	lang := strings.ToLower(v.Config.Language)
	if lang == "" {
		lang = "auto"
	}
	// the configured folder first, then the app's default folder, so a vault whose config.json
	// still points at an old location keeps working after the models move
	dirs := []string{v.Config.ModelsDir, defaultModelsDir()}
	heard := ""
	if lang == "auto" {
		if heard = routeLanguage(v, dirs, wav, m.DurationS); heard != "" {
			lang = heard
		}
	}
	modelFile := v.Config.TurboModel
	if lang == "sv" {
		modelFile = v.Config.KBModel
	}
	modelPath, vadPath := "", ""
	for _, d := range dirs {
		mp, vp := filepath.Join(d, modelFile), filepath.Join(d, v.Config.VADModel)
		if _, err := os.Stat(mp); err != nil {
			continue
		}
		if _, err := os.Stat(vp); err != nil {
			continue
		}
		modelPath, vadPath = mp, vp
		break
	}
	if modelPath == "" && modelFile == speechModels[0].Name {
		// first use: fetch the general model and the voice detector into the app's folder
		dir := defaultModelsDir()
		for _, mf := range speechModels {
			setProgress("downloading", 0, 0, 0)
			if err := ensureModel(dir, mf, func(string, ...any) {}); err != nil {
				return nil, fmt.Errorf("the speech model could not be downloaded: %w", err)
			}
		}
		modelPath, vadPath = filepath.Join(dir, modelFile), filepath.Join(dir, v.Config.VADModel)
	}
	if modelPath == "" {
		return nil, fmt.Errorf("model files %s and %s not found in %s or %s (run `poiesis setup --swedish`, or set models_dir in config.json / POIESIS_MODELS)", modelFile, v.Config.VADModel, dirs[0], dirs[1])
	}
	modelName := strings.TrimSuffix(modelFile, ".bin")
	base := strings.TrimSuffix(v.Path(m.Path), filepath.Ext(m.Path)) + ".words." + modelName + "." + lang
	cache := base + ".json"
	start := time.Now()
	if _, err := os.Stat(cache); err != nil {
		args := []string{"-m", modelPath, "-f", wav, "-l", lang, "--vad", "-vm", vadPath,
			"-ml", "1", "-sow", "-ojf", "-pp", "-t", fmt.Sprint(v.Config.Threads), "-of", base}
		setProgress("transcribing", 0, 0, 0)
		if names != "" {
			args = append(args, "--prompt", names)
		}
		out, err := runReporting("transcribing", whisperCmd(v, args))
		if err != nil && (strings.Contains(string(out), "failed to initialize VAD context") || strings.Contains(err.Error(), "signal")) {
			// Seen when another process (Ollama) holds a model in GPU memory: the voice
			// detection model cannot start. Retry once without voice detection, loudly.
			var noVAD []string
			skip := false
			for _, a := range args {
				if a == "--vad" {
					continue
				}
				if a == "-vm" {
					skip = true
					continue
				}
				if skip {
					skip = false
					continue
				}
				noVAD = append(noVAD, a)
			}
			var out2 []byte
			first := err
			out2, err = runReporting("transcribing", whisperCmd(v, noVAD))
			out = append(out, []byte("\n[poiesis] voice detection failed ("+first.Error()+"); retried without it (silence hallucinations possible)\n")...)
			out = append(out, out2...)
		}
		if err != nil {
			// whisper.cpp 1.8.4 with the Homebrew ggml Metal backend can abort in a destructor
			// at process exit after writing its output. Accept the run if the JSON is complete.
			if b, rerr := os.ReadFile(cache); rerr == nil && json.Valid(b) {
				out = append(out, []byte("\n[poiesis] whisper-cli exited with an error after writing its output; output accepted: "+err.Error()+"\n")...)
			} else {
				return nil, fmt.Errorf("whisper-cli: %w\n%s", err, tail(string(out), 1200))
			}
		}
		_ = os.WriteFile(base+".log", out, 0o644)
	}
	b, err := os.ReadFile(cache)
	if err != nil {
		return nil, err
	}
	var wj whisperJSON
	if err := json.Unmarshal(b, &wj); err != nil {
		return nil, fmt.Errorf("whisper json: %w", err)
	}
	t := &Transcript{Model: modelName, Language: lang, Transcribr: "whisper.cpp", Seconds: time.Since(start).Seconds()}
	if lang == "auto" {
		t.Detected = wj.Result.Language
	}
	if heard != "" {
		t.Detected = heard
	}
	for _, seg := range wj.Transcription {
		txt := strings.TrimSpace(seg.Text)
		if txt == "" {
			continue
		}
		t.Words = append(t.Words, Word{Text: txt, Start: float64(seg.Offsets.From) / 1000, End: float64(seg.Offsets.To) / 1000})
	}
	return t, nil
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

// Line is a readable transcript line built from words: for the entry page and the extractor.
type Line struct {
	Start float64
	End   float64
	Text  string
}

var sentenceEnd = regexp.MustCompile(`[.!?…]["»)]?$`)

// Lines groups words into readable lines: a new line on a pause over 0.8 s, after a sentence
// end once the line has six words, or at fourteen words.
func Lines(words []Word) []Line {
	var lines []Line
	var cur []Word
	flush := func() {
		if len(cur) == 0 {
			return
		}
		parts := make([]string, len(cur))
		for i, w := range cur {
			parts[i] = w.Text
		}
		lines = append(lines, Line{Start: cur[0].Start, End: cur[len(cur)-1].End, Text: strings.Join(parts, " ")})
		cur = nil
	}
	for i, w := range words {
		if len(cur) > 0 {
			gap := w.Start - cur[len(cur)-1].End
			last := cur[len(cur)-1].Text
			if gap > 0.8 || len(cur) >= 14 || (len(cur) >= 6 && sentenceEnd.MatchString(last)) {
				flush()
			}
		}
		cur = append(cur, w)
		if i == len(words)-1 {
			flush()
		}
	}
	return lines
}

// Window is a stretch of transcript sent to the extractor in one call.
type Window struct {
	Index int
	Start float64
	End   float64
	Lines []Line
}

// Windows cuts lines into 2-4 minute windows on pauses over 1.5 s (hard cut at 4 minutes).
func Windows(lines []Line) []Window {
	var ws []Window
	var cur []Line
	flush := func() {
		if len(cur) == 0 {
			return
		}
		ws = append(ws, Window{Index: len(ws), Start: cur[0].Start, End: cur[len(cur)-1].End, Lines: cur})
		cur = nil
	}
	for _, l := range lines {
		if len(cur) > 0 {
			span := l.End - cur[0].Start
			gap := l.Start - cur[len(cur)-1].End
			if (span >= 120 && gap > 1.5) || span >= 240 {
				flush()
			}
		}
		cur = append(cur, l)
	}
	flush()
	return ws
}

// normalizeText lowercases, drops punctuation and collapses spaces, for quote matching.
func normalizeText(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
		} else if !space && b.Len() > 0 {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// LocateQuote finds the quote in the word list and returns the exact start and end seconds.
func LocateQuote(words []Word, quote string) (start, end float64, ok bool) {
	q := strings.Fields(normalizeText(quote))
	if len(q) == 0 {
		return 0, 0, false
	}
	norm := make([]string, len(words))
	for i, w := range words {
		norm[i] = normalizeText(w.Text)
	}
	// words may normalise to several tokens ("pizzeria/bageri"); match on the joined stream
	var stream []string
	var owner []int
	for i, n := range norm {
		for _, tok := range strings.Fields(n) {
			stream = append(stream, tok)
			owner = append(owner, i)
		}
	}
	for i := 0; i+len(q) <= len(stream); i++ {
		match := true
		for j := range q {
			if stream[i+j] != q[j] {
				match = false
				break
			}
		}
		if match {
			return words[owner[i]].Start, words[owner[i+len(q)-1]].End, true
		}
	}
	return 0, 0, false
}

func mmss(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	m := int(sec) / 60
	s := int(sec) % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

// whisperCmd runs whisper-cli. The copy inside the app finds ggml's backends beside itself.
func whisperCmd(v *Vault, args []string) *exec.Cmd {
	bin := findTool(v.Config.WhisperBin)
	c := exec.Command(bin, args...)
	return c
}

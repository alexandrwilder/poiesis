package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

const promptVersion = "extract-v5"

var claimKinds = []string{"event", "state", "belief", "decision", "intention", "prediction", "question", "lesson"}

// Claim is one line in episodes/<id>.claims.jsonl: the thirteen fields of the design.
type Claim struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Text        string   `json:"text"`
	Quote       string   `json:"quote"`
	About       []string `json:"about"`
	StatedAt    string   `json:"stated_at"`
	ValidTo     *string  `json:"valid_to"`
	Supersedes  *string  `json:"supersedes"`
	Due         *string  `json:"due"`
	Outcome     *string  `json:"outcome"`
	Source      Source   `json:"source"`
	Confidence  float64  `json:"confidence"`
	ExtractedAt string   `json:"extracted_at"`
	Extractor   string   `json:"extractor"`
}

type Source struct {
	Episode string  `json:"episode"`
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
}

// rawClaim is what the model returns, before validation. The model names what a claim is
// about in plain words, e.g. "Sara (person)", "the oven (thing)", "mindset (theme)"; the code
// resolves names to entity pages, so the model never picks from a list (small models
// over-pick from lists, and nested objects made the 4B model loop).
type rawClaim struct {
	Kind       string   `json:"kind"`
	Text       string   `json:"text"`
	Quote      string   `json:"quote"`
	About      []string `json:"about"`
	Due        string   `json:"due"`
	Confidence float64  `json:"confidence"`
	Start      float64  `json:"start"`
	End        float64  `json:"end"`
}

// ExtractResult is the model's answer for one window.
type ExtractResult struct {
	Claims []rawClaim   `json:"claims"`
	Stats  ExtractStats `json:"-"`
}

// ExtractStats is what a call cost, so the log can say why an ingest took as long as it did.
type ExtractStats struct {
	Calls        int
	PromptTokens int
	OutputTokens int
	Seconds      float64
}

func (s *ExtractStats) add(o ExtractStats) {
	s.Calls += o.Calls
	s.PromptTokens += o.PromptTokens
	s.OutputTokens += o.OutputTokens
	s.Seconds += o.Seconds
}

func (s ExtractStats) String() string {
	rate := 0.0
	if s.Seconds > 0 {
		rate = float64(s.OutputTokens) / s.Seconds
	}
	return fmt.Sprintf("%d call(s), %d in / %d out tokens, %.1f tok/s, %.0f s", s.Calls, s.PromptTokens, s.OutputTokens, rate, s.Seconds)
}

// ExtractRequest is everything the extractor may know about one window.
type ExtractRequest struct {
	EpisodeID   string
	RecordedAt  time.Time
	Day         int
	Mission     string
	Language    string
	Detected    string
	Window      Window
	WindowCount int
	Entities    map[string]*Entity
}

type Extractor interface {
	Name() string
	Extract(ctx context.Context, req ExtractRequest) (ExtractResult, error)
	// Release frees whatever the extractor holds on the machine (a local model in GPU memory).
	// whisper's voice detection fails to start while Ollama still holds a model, so ingest
	// calls this before transcribing and after extracting.
	Release(ctx context.Context)
}

// extractionSchema is the JSON schema every extractor must satisfy (kept under thirty fields).
var extractionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"claims": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":       map[string]any{"type": "string", "enum": claimKinds},
					"text":       map[string]any{"type": "string"},
					"quote":      map[string]any{"type": "string"},
					"about":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"due":        map[string]any{"type": "string"},
					"confidence": map[string]any{"type": "number"},
					"start":      map[string]any{"type": "number"},
					"end":        map[string]any{"type": "number"},
				},
				"required":             []string{"kind", "text", "quote", "about", "due", "confidence", "start", "end"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"claims"},
	"additionalProperties": false,
}

const systemPrompt = `You turn one window of a spoken video journal into claims for a personal knowledge graph. The speaker talks to a camera about their own life, work and thinking, often mixing Swedish and English.

A claim is one thing the speaker said that matters, as one sentence. Kinds, pick exactly one:
- event: something happened, or a fact about the world (a delivery is late, a meeting took place).
- state: how the speaker themselves is right now (mood, energy, sleep, stress, worry). Never a fact about the world.
- belief: what the speaker holds true or thinks, including what they think about what someone else said.
- decision: a choice the speaker made, with its reason if given.
- intention: something the speaker commits to doing, with a time window if given.
- prediction: a forecast the speaker makes that could be scored later.
- question: an open question the speaker is holding.
- lesson: something the speaker says they learned.

Rules:
- "text": the claim as one clear first-person sentence in English, faithful to what was said. Do not add interpretation.
- "quote": the exact words from the transcript that support the claim, copied verbatim in the original language, 3 to 25 words, no paraphrase, no ellipsis. It must appear in the transcript exactly.
- "about": what the claim is about, as one to three short names, usually one, each with its kind in parentheses: "Sara (person)", "the bakery (project)", "Uppsala (place)", "the oven (thing)", "mindset (theme)", "running (habit)". Ask "which page should this claim appear on?" and answer with the most specific one. A thing is used only when the claim is about that thing ("the oven" for "the oven is late", not for "we can start small"). Habits, rules and attitudes go on a theme or habit page. A claim about what a named person said, thinks or wants is about that person. Never use the person talking as a topic: the whole log is theirs. Never attach a claim to a thing or project just because it was mentioned nearby.
- "start" and "end": seconds, taken from the transcript line timings around the quote.
- "due": for intention and prediction only, an ISO date (YYYY-MM-DD) if the speaker gives a date or a clear window (this week, in October); otherwise "".
- "confidence": 0 to 1, how sure you are the claim is what the speaker meant.
- A person who is named and does or thinks something (for example "Sara thinks we should wait") gets a claim about that person.
- Go through the window in order from the first line to the last; the beginning matters as much as the end. Each claim once; no repeats.
- Every sentence that states a fact, a plan, a decision, an opinion, a feeling, a question or a lesson becomes a claim; do not skip one because it seems small. Skip only filler, greetings, jokes about the recording, and half-sentences ("and now I will freestyle a bit"). Never more than fifteen claims per window.
- Never invent. If the window has nothing that matters, return an empty list.`

func buildUserPrompt(req ExtractRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Recording %s, %s, day %d.", req.EpisodeID, req.RecordedAt.Format("2006-01-02 (Monday) 15:04"), req.Day)
	fmt.Fprintf(&b, " Language setting: %s", req.Language)
	if req.Detected != "" {
		fmt.Fprintf(&b, " (detected %s)", req.Detected)
	}
	b.WriteString(".\n")
	// No list of known entities: the model names what a claim is about and the code matches
	// the name to an existing page (exact or alias) or creates one. Small models over-pick
	// from lists; naming is what they are good at.
	fmt.Fprintf(&b, "\nTranscript, window %d of %d, %s to %s. Each line: [start–end seconds] words.\n",
		req.Window.Index+1, req.WindowCount, mmss(req.Window.Start), mmss(req.Window.End))
	for _, l := range req.Window.Lines {
		fmt.Fprintf(&b, "[%.1f–%.1f] %s\n", l.Start, l.End, l.Text)
	}
	return b.String()
}

// NewExtractor picks the backend from the vault config.
func NewExtractor(cfg Config, claimsFile string) (Extractor, error) {
	switch cfg.Extractor {
	case "claude", "":
		model := cfg.Model
		if model == "" {
			model = "claude-opus-5"
		}
		return &claudeExtractor{model: model, client: anthropic.NewClient()}, nil
	case "ollama":
		model := cfg.Model
		if model == "" {
			model = "qwen3.5:4b" // measured 2026-09-02 on the mixed clip: best claims per second that fits a laptop
		}
		return &ollamaExtractor{model: model, url: cfg.OllamaURL}, nil
	case "file":
		if claimsFile == "" {
			return nil, fmt.Errorf("--extractor file needs --claims-file")
		}
		return &fileExtractor{path: claimsFile}, nil
	}
	return nil, fmt.Errorf("unknown extractor %q (claude, ollama, file)", cfg.Extractor)
}

// claudeExtractor: one Messages call per window with a JSON-schema output format.
type claudeExtractor struct {
	model  string
	client anthropic.Client
}

func (c *claudeExtractor) Name() string { return c.model + "/" + promptVersion }

func (c *claudeExtractor) Release(ctx context.Context) {}

func (c *claudeExtractor) Extract(ctx context.Context, req ExtractRequest) (ExtractResult, error) {
	var res ExtractResult
	started := time.Now()
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 8000,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(buildUserPrompt(req))),
		},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffortHigh,
			Format: anthropic.JSONOutputFormatParam{Schema: extractionSchema},
		},
	})
	if err != nil {
		return res, fmt.Errorf("claude: %w", err)
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return res, fmt.Errorf("claude declined the request (%s)", resp.StopDetails.Category)
	}
	var text string
	for _, block := range resp.Content {
		if b, ok := block.AsAny().(anthropic.TextBlock); ok {
			text += b.Text
		}
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		return res, fmt.Errorf("claude returned no valid JSON: %w", err)
	}
	res.Stats = ExtractStats{Calls: 1, PromptTokens: int(resp.Usage.InputTokens), OutputTokens: int(resp.Usage.OutputTokens), Seconds: time.Since(started).Seconds()}
	return res, nil
}

// ollamaExtractor: the same prompt through a local model with a JSON-schema format.
type ollamaExtractor struct {
	model string
	url   string
}

func (o *ollamaExtractor) Name() string { return "ollama:" + o.model + "/" + promptVersion }

// Release asks Ollama to unload the model now (keep_alive 0) so the GPU is free for whisper.
func (o *ollamaExtractor) Release(ctx context.Context) {
	payload, _ := json.Marshal(map[string]any{"model": o.model, "keep_alive": 0})
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(o.url, "/")+"/api/generate", bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

func (o *ollamaExtractor) Extract(ctx context.Context, req ExtractRequest) (ExtractResult, error) {
	var res ExtractResult
	body := map[string]any{
		"model": o.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": buildUserPrompt(req)},
		},
		"stream":  false,
		"format":  extractionSchema,
		"think":   false,
		"options": map[string]any{"temperature": 0, "num_ctx": 16384},
	}
	payload, _ := json.Marshal(body)
	url := strings.TrimRight(o.url, "/") + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return res, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(httpReq)
	if err != nil {
		return res, fmt.Errorf("ollama at %s: %w (is it running?)", o.url, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return res, fmt.Errorf("ollama: %s: %s", resp.Status, tail(string(out), 400))
	}
	var chat struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		PromptEvalCount int64 `json:"prompt_eval_count"`
		EvalCount       int64 `json:"eval_count"`
		EvalDuration    int64 `json:"eval_duration"`  // nanoseconds spent generating
		TotalDuration   int64 `json:"total_duration"` // nanoseconds for the whole call
	}
	if err := json.Unmarshal(out, &chat); err != nil {
		return res, fmt.Errorf("ollama response: %w", err)
	}
	if err := json.Unmarshal([]byte(chat.Message.Content), &res); err != nil {
		return res, fmt.Errorf("ollama returned no valid JSON: %w\n%s", err, tail(chat.Message.Content, 400))
	}
	res.Stats = ExtractStats{Calls: 1, PromptTokens: int(chat.PromptEvalCount), OutputTokens: int(chat.EvalCount), Seconds: float64(chat.TotalDuration) / 1e9}
	return res, nil
}

// fileExtractor reads a prepared result (for tests and for extraction done elsewhere).
type fileExtractor struct{ path string }

func (f *fileExtractor) Name() string { return "file/" + promptVersion }

func (f *fileExtractor) Release(ctx context.Context) {}

func (f *fileExtractor) Extract(ctx context.Context, req ExtractRequest) (ExtractResult, error) {
	var res ExtractResult
	b, err := os.ReadFile(f.path)
	if err != nil {
		return res, err
	}
	if req.Window.Index > 0 { // a file holds one window's worth
		return res, nil
	}
	return res, json.Unmarshal(b, &res)
}

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The MCP server: a librarian between any AI and the vault. Four read-only verbs over
// stdio, local only. It never writes, holds no opinions, returns nothing without an
// episode id and seconds, and caps every answer.

const (
	mcpMaxChars   = 16000
	mcpMaxHits    = 50
	mcpDefaultHit = 20
	mcpWindowS    = 40.0
)

type orientIn struct{}

type searchIn struct {
	Query   string `json:"query" jsonschema:"words to look for in claims and in the transcript; all words must match"`
	From    string `json:"from,omitempty" jsonschema:"only entries on or after this date, YYYY-MM-DD"`
	To      string `json:"to,omitempty" jsonschema:"only entries on or before this date, YYYY-MM-DD"`
	Kind    string `json:"kind,omitempty" jsonschema:"only claims of this kind: event, state, belief, decision, intention, prediction, question, lesson"`
	Entity  string `json:"entity,omitempty" jsonschema:"only claims about this entity id, e.g. sara or bakery"`
	Mission string `json:"mission,omitempty" jsonschema:"only entries recorded under this mission id"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum number of hits, default 20, at most 50"`
}

type readIn struct {
	ID string `json:"id" jsonschema:"what to read: an entity id (sara), an entry id (2026-09-02-a), or one of index, schema, log"`
}

type momentIn struct {
	Episode string  `json:"episode" jsonschema:"the entry id, e.g. 2026-09-02-a"`
	T       float64 `json:"t" jsonschema:"seconds into the recording"`
	WindowS float64 `json:"window_s,omitempty" jsonschema:"how many seconds of transcript to return around t, default 40"`
}

func runMCP(v *Vault) error {
	server := mcp.NewServer(&mcp.Implementation{Name: "poiesis", Version: version}, nil)
	ro := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(server, &mcp.Tool{Name: "orient", Annotations: ro,
		Description: "Where am I? Returns the vault's map of content: entries, entities by mentions, open loops. Read this first, then drill with read, search and moment."},
		func(ctx context.Context, req *mcp.CallToolRequest, in orientIn) (*mcp.CallToolResult, any, error) {
			return text(v.orient()), nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "search", Annotations: ro,
		Description: "Where did I say something about X? Full-text search over claims and the transcript, with optional date, kind, entity and mission filters. Every hit carries the entry id and the seconds, so it can be played back or quoted with its source."},
		func(ctx context.Context, req *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, any, error) {
			s, err := v.mcpSearch(in)
			if err != nil {
				return nil, nil, err
			}
			return text(s), nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "read", Annotations: ro,
		Description: "Give me this page: an entity page (timeline of claims), an entry page (claims and transcript), or index, schema, log. Long pages are cut at 16000 characters; use moment for a precise span."},
		func(ctx context.Context, req *mcp.CallToolRequest, in readIn) (*mcp.CallToolResult, any, error) {
			s, err := v.mcpRead(in.ID)
			if err != nil {
				return nil, nil, err
			}
			return text(s), nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "moment", Annotations: ro,
		Description: "Show me that second: the verbatim transcript around a time in an entry, the claims made in that span, and the playback link."},
		func(ctx context.Context, req *mcp.CallToolRequest, in momentIn) (*mcp.CallToolResult, any, error) {
			s, err := v.mcpMoment(in)
			if err != nil {
				return nil, nil, err
			}
			return text(s), nil, nil
		})

	return server.Run(context.Background(), &mcp.StdioTransport{})
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func capText(s string, n int, hint string) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("\n\n[cut here: %d more characters. %s]", len(s)-n, hint)
}

func (v *Vault) orient() string {
	b, err := os.ReadFile(v.Path("_index.md"))
	if err != nil {
		return "This vault has no entries yet. Pages appear after the first recording is ingested. Read `schema` for the layout."
	}
	head := "How to read this vault: this index first; then read(entity) for a timeline, read(entry) for one recording, search(words) to find moments, moment(entry, t) for the exact words around a second. Quote nothing without its entry id and seconds.\n\n"
	return capText(head+string(b), mcpMaxChars, "use read(entity) or search for the rest")
}

func (v *Vault) mcpRead(id string) (string, error) {
	id = strings.TrimSpace(id)
	var path string
	switch strings.ToLower(strings.TrimPrefix(id, "_")) {
	case "index", "index.md":
		path = v.Path("_index.md")
	case "schema", "schema.md":
		path = v.Path("SCHEMA.md")
	case "log", "log.md":
		b, err := os.ReadFile(v.Path("_log.md"))
		if err != nil {
			return "", fmt.Errorf("no log yet")
		}
		lines := strings.Split(string(b), "\n")
		if len(lines) > 120 {
			lines = lines[len(lines)-120:]
		}
		return strings.Join(lines, "\n"), nil
	default:
		safe := filepath.Base(strings.TrimSuffix(id, ".md"))
		if p := v.Path("entities", safe+".md"); fileExists(p) {
			path = p
		} else if p := v.Path("episodes", safe+".md"); fileExists(p) {
			path = p
		} else if resolved := v.ResolveEntity(id); resolved != "" && fileExists(v.Path("entities", resolved+".md")) {
			path = v.Path("entities", resolved+".md")
		} else {
			return "", fmt.Errorf("no page named %q; try search, or read(index) for the list of entities and entries", id)
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return capText(string(b), mcpMaxChars, "use moment(entry, t) for a specific span"), nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func (v *Vault) mcpSearch(in searchIn) (string, error) {
	d, err := loadTUIData(v)
	if err != nil {
		return "", err
	}
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(in.Query)))
	if len(terms) == 0 {
		return "", fmt.Errorf("give at least one word to search for")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = mcpDefaultHit
	}
	if limit > mcpMaxHits {
		limit = mcpMaxHits
	}
	inRange := func(ep Episode) bool {
		day := ep.RecordedAt[:10]
		if in.From != "" && day < in.From {
			return false
		}
		if in.To != "" && day > in.To {
			return false
		}
		if in.Mission != "" && ep.Mission != slugify(in.Mission) {
			return false
		}
		return true
	}
	matches := func(hay string) bool {
		hay = strings.ToLower(hay)
		for _, t := range terms {
			if !strings.Contains(hay, t) {
				return false
			}
		}
		return true
	}
	var b strings.Builder
	var claimHits []Claim
	for _, c := range d.claims {
		ep, ok := d.byID[c.Source.Episode]
		if !ok || !inRange(ep) {
			continue
		}
		if in.Kind != "" && c.Kind != in.Kind {
			continue
		}
		if in.Entity != "" && !contains(c.About, slugify(in.Entity)) {
			continue
		}
		if matches(c.Text + " " + c.Quote + " " + strings.Join(c.About, " ")) {
			claimHits = append(claimHits, c)
		}
	}
	type lineHit struct {
		ep   Episode
		line Line
	}
	var lineHits []lineHit
	if in.Kind == "" && in.Entity == "" {
		for _, ep := range d.entries {
			if !inRange(ep) {
				continue
			}
			lines, err := transcriptLines(v, ep)
			if err != nil {
				continue
			}
			for _, l := range lines {
				if matches(l.Text) {
					lineHits = append(lineHits, lineHit{ep, l})
				}
			}
		}
		sort.SliceStable(lineHits, func(i, j int) bool { return lineHits[i].ep.RecordedAt > lineHits[j].ep.RecordedAt })
	}
	fmt.Fprintf(&b, "%d claim(s) and %d transcript line(s) match %q", len(claimHits), len(lineHits), in.Query)
	if len(claimHits)+len(lineHits) > limit {
		fmt.Fprintf(&b, "; showing the newest %d", limit)
	}
	b.WriteString(".\n")
	shown := 0
	if len(claimHits) > 0 {
		b.WriteString("\nClaims (date · kind · text — \"quote\" · entry t=seconds · about):\n")
		for _, c := range claimHits {
			if shown >= limit {
				break
			}
			fmt.Fprintf(&b, "- %s · %s · %s — \"%s\" · %s t=%.1f (poiesis://%s?t=%.1f) · about: %s\n",
				c.StatedAt[:10], c.Kind, c.Text, c.Quote, c.Source.Episode, c.Source.Start, c.Source.Episode, c.Source.Start, strings.Join(c.About, ", "))
			shown++
		}
	}
	if len(lineHits) > 0 && shown < limit {
		b.WriteString("\nTranscript lines (entry t=seconds: words):\n")
		for _, h := range lineHits {
			if shown >= limit {
				break
			}
			fmt.Fprintf(&b, "- %s t=%.1f: %s (poiesis://%s?t=%.1f)\n", h.ep.ID, h.line.Start, h.line.Text, h.ep.ID, h.line.Start)
			shown++
		}
	}
	if shown == 0 {
		b.WriteString("\nNothing found. Try fewer words, or read(index) to see what the vault holds.\n")
	}
	return capText(b.String(), mcpMaxChars, "narrow the search with from/to, kind or entity"), nil
}

func (v *Vault) mcpMoment(in momentIn) (string, error) {
	d, err := loadTUIData(v)
	if err != nil {
		return "", err
	}
	ep, ok := d.byID[strings.TrimSpace(in.Episode)]
	if !ok {
		return "", fmt.Errorf("no entry %q; entry ids look like 2026-09-02-a, see read(index)", in.Episode)
	}
	w := in.WindowS
	if w <= 0 {
		w = mcpWindowS
	}
	lines, err := transcriptLines(v, ep)
	if err != nil {
		return "", fmt.Errorf("transcript of %s not readable: %w", ep.ID, err)
	}
	lo, hi := in.T-w/2, in.T+w/2
	var b strings.Builder
	fmt.Fprintf(&b, "%s · day %d · recorded %s · %s long · around %s (t=%.1f)\nplay: poiesis://%s?t=%.1f · file: %s\n\nTranscript:\n",
		ep.ID, ep.Day, strings.Replace(ep.RecordedAt[:16], "T", " ", 1), mmss(ep.DurationS), mmss(in.T), in.T, ep.ID, in.T, ep.Media)
	n := 0
	for _, l := range lines {
		if l.End < lo || l.Start > hi {
			continue
		}
		mark := "  "
		if l.Start <= in.T && in.T <= l.End+0.5 {
			mark = "▶ "
		}
		fmt.Fprintf(&b, "%s[%s] %s\n", mark, mmss(l.Start), l.Text)
		n++
	}
	if n == 0 {
		fmt.Fprintf(&b, "  (no words between %s and %s; the recording is %s long)\n", mmss(max0(lo)), mmss(hi), mmss(ep.DurationS))
	}
	var cs []Claim
	for _, c := range d.byEntry[ep.ID] {
		if c.Source.Start >= lo && c.Source.Start <= hi {
			cs = append(cs, c)
		}
	}
	if len(cs) > 0 {
		b.WriteString("\nClaims in this span:\n")
		for _, c := range cs {
			fmt.Fprintf(&b, "- %s · t=%.1f · %s — \"%s\" · about: %s\n", c.Kind, c.Source.Start, c.Text, c.Quote, strings.Join(c.About, ", "))
		}
	}
	return capText(b.String(), mcpMaxChars, "use a smaller window_s"), nil
}

func max0(f float64) float64 {
	if f < 0 {
		return 0
	}
	return f
}

// connectText prints the one-line setup for the common AI clients.
func connectText(v *Vault) string {
	exe, _ := os.Executable()
	exe, _ = filepath.Abs(exe)
	vault := v.Root
	return fmt.Sprintf(`Connect your AI to this vault. The server runs on your machine and only reads.

Claude Code (one command):
  claude mcp add poiesis -- %q mcp --vault %q

Claude Desktop: add to claude_desktop_config.json (Settings → Developer → Edit Config):
  {
    "mcpServers": {
      "poiesis": { "command": %q, "args": ["mcp", "--vault", %q] }
    }
  }

Cursor: the same object in .cursor/mcp.json (project) or ~/.cursor/mcp.json (global).

Then ask: "Orient yourself in my log, then tell me what I said about X and play the moment."
The verbs the AI gets: orient, search, read, moment. Nothing writes.
`, exe, vault, exe, vault)
}

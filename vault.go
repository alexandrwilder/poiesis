package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config lives in <vault>/config.json. Every field has a default so the file is optional.
type Config struct {
	Language      string `json:"language"`    // auto | sv | en
	Extractor     string `json:"extractor"`   // claude | ollama | file
	Model         string `json:"model"`       // claude-opus-5 for claude, qwen3:8b for ollama
	ModelsDir     string `json:"models_dir"`  // where the speech model files live
	WhisperBin    string `json:"whisper_bin"` // whisper-cli
	FFmpegBin     string `json:"ffmpeg_bin"`  // ffmpeg
	FFprobeBin    string `json:"ffprobe_bin"` // ffprobe
	OllamaURL     string `json:"ollama_url"`  // http://localhost:11434
	TurboModel    string `json:"turbo_model"` // file name inside models_dir
	KBModel       string `json:"kb_model"`    // file name inside models_dir
	VADModel      string `json:"vad_model"`   // file name inside models_dir
	Threads       int    `json:"threads"`
	CaptureDevice string `json:"capture_device"` // ffmpeg input device, e.g. "0:0" on a Mac
	LastMission   string `json:"last_mission"`   // remembered mission line on the record screen
	Reflection    string `json:"reflection"`     // on (default) | off: the camera while the record screen is open
	Theme         string `json:"theme"`          // ember (default) | frost | phosphor | mono | prism | dusk
	Video         string `json:"video"`          // clear | light | styled; empty = clear, or light when the machine struggles
	MaxEntryS     int    `json:"max_entry_s"`    // an entry stops itself after this many seconds
	UpdateCheck   string `json:"update_check"`   // daily (default) | off: asks the release page for the newest version
}

func defaultConfig() Config {
	return Config{
		Language:      "auto",
		Extractor:     "ollama", // on this computer; a cloud AI is only ever the person's own choice
		Model:         "",
		ModelsDir:     defaultModelsDir(),
		WhisperBin:    "whisper-cli",
		FFmpegBin:     "ffmpeg",
		FFprobeBin:    "ffprobe",
		OllamaURL:     "http://localhost:11434",
		TurboModel:    "ggml-large-v3-turbo-q5_0.bin",
		KBModel:       "kb-whisper-large-q5_0.bin",
		VADModel:      "ggml-silero-v5.1.2.bin",
		Threads:       8,
		CaptureDevice: defaultCaptureDevice(),
		Reflection:    "on",
		Theme:         "ember",
		Video:         "",
		MaxEntryS:     180,
		UpdateCheck:   "daily",
	}
}

// appStateDir is the app's own folder: model files, the running window's id, and the
// one-line command file the menu bar item writes. Never inside the vault.
func appStateDir() string {
	return filepath.Dir(defaultModelsDir())
}

// defaultModelsDir is the app's own data folder, never inside the vault.
func defaultModelsDir() string {
	if d := os.Getenv("POIESIS_MODELS"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Poiesis", "models")
	case "windows":
		if a := os.Getenv("APPDATA"); a != "" {
			return filepath.Join(a, "Poiesis", "models")
		}
		return filepath.Join(home, "Poiesis", "models")
	default:
		if x := os.Getenv("XDG_DATA_HOME"); x != "" {
			return filepath.Join(x, "Poiesis", "models")
		}
		return filepath.Join(home, ".local", "share", "poiesis", "models")
	}
}

// Entity is the frontmatter of entities/<id>.md.
type Entity struct {
	ID        string   `yaml:"id"`
	Kind      string   `yaml:"kind"`
	Aliases   []string `yaml:"aliases"`
	FirstSeen string   `yaml:"first_seen,omitempty"`
	LastSeen  string   `yaml:"last_seen,omitempty"`
	Mentions  int      `yaml:"mentions"`
}

type Vault struct {
	Root     string
	Config   Config
	Entities map[string]*Entity // by id
}

var entityKinds = []string{"person", "project", "mission", "theme", "place", "org", "habit"}

func OpenVault(root string) (*Vault, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	v := &Vault{Root: root, Config: defaultConfig(), Entities: map[string]*Entity{}}
	for _, d := range []string{"", "inbox", "raw", "episodes", "entities", "periods"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return nil, err
		}
	}
	if b, err := os.ReadFile(v.Path("config.json")); err == nil {
		if err := json.Unmarshal(b, &v.Config); err != nil {
			return nil, fmt.Errorf("config.json: %w", err)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := v.SaveConfig(); err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(v.Path("SCHEMA.md")); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(v.Path("SCHEMA.md"), []byte(schemaMD), 0o644); err != nil {
			return nil, err
		}
	}
	if err := v.loadEntities(); err != nil {
		return nil, err
	}
	return v, nil
}

func (v *Vault) Path(parts ...string) string {
	return filepath.Join(append([]string{v.Root}, parts...)...)
}

func (v *Vault) SaveConfig() error {
	b, err := json.MarshalIndent(v.Config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(v.Path("config.json"), append(b, '\n'), 0o644)
}

func (v *Vault) InboxFiles() ([]string, error) {
	entries, err := os.ReadDir(v.Path("inbox"))
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".mp4", ".mov", ".m4v", ".mkv", ".webm", ".m4a", ".wav", ".mp3":
			files = append(files, v.Path("inbox", e.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

// loadEntities reads the frontmatter of every entity page.
func (v *Vault) loadEntities() error {
	entries, err := os.ReadDir(v.Path("entities"))
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		b, err := os.ReadFile(v.Path("entities", e.Name()))
		if err != nil {
			return err
		}
		fm, _ := splitFrontmatter(string(b))
		var ent Entity
		if err := yaml.Unmarshal([]byte(fm), &ent); err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		if ent.ID == "" {
			ent.ID = strings.TrimSuffix(e.Name(), ".md")
		}
		v.Entities[ent.ID] = &ent
	}
	return nil
}

// splitFrontmatter returns the YAML between the first pair of --- lines and the body after it.
func splitFrontmatter(s string) (fm string, body string) {
	if !strings.HasPrefix(s, "---\n") {
		return "", s
	}
	rest := s[4:]
	i := strings.Index(rest, "\n---\n")
	if i < 0 {
		return "", s
	}
	return rest[:i], rest[i+5:]
}

// ResolveEntity maps a name or id the extractor used onto an existing entity id.
// Exact id match first, then alias match, both after normalisation. "" if unknown.
func (v *Vault) ResolveEntity(name string) string {
	n := normalizeKey(name)
	if n == "" {
		return ""
	}
	if _, ok := v.Entities[n]; ok {
		return n
	}
	for id, e := range v.Entities {
		if normalizeKey(id) == n {
			return id
		}
		for _, a := range e.Aliases {
			if normalizeKey(a) == n {
				return id
			}
		}
	}
	return ""
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slugify: "Familjetapeter AB" -> "familjetapeter-ab", "Uppsala" -> "uppsala".
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("å", "a", "ä", "a", "ö", "o", "é", "e", "ü", "u", "ø", "o", "æ", "ae").Replace(s)
	s = nonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func normalizeKey(s string) string { return slugify(s) }

// NamesHint builds the vocabulary sent to the transcriber: aliases and names of every entity.
func (v *Vault) NamesHint(extra ...string) string {
	seen := map[string]bool{}
	var names []string
	add := func(n string) {
		n = strings.TrimSpace(n)
		if n == "" || seen[strings.ToLower(n)] {
			return
		}
		seen[strings.ToLower(n)] = true
		names = append(names, n)
	}
	for _, n := range extra {
		add(n)
	}
	ids := make([]string, 0, len(v.Entities))
	for id := range v.Entities {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return v.Entities[ids[i]].Mentions > v.Entities[ids[j]].Mentions })
	for _, id := range ids {
		e := v.Entities[id]
		if len(e.Aliases) > 0 {
			add(e.Aliases[0])
		} else {
			add(humanize(id))
		}
	}
	hint := strings.Join(names, ", ")
	if len(hint) > 600 { // whisper's prompt window is small; keep the most-mentioned names
		hint = hint[:600]
		if i := strings.LastIndex(hint, ","); i > 0 {
			hint = hint[:i]
		}
	}
	if hint != "" {
		hint += "."
	}
	return hint
}

// EpisodeDates returns every episode id in the vault (sorted).
func (v *Vault) EpisodeIDs() ([]string, error) {
	entries, err := os.ReadDir(v.Path("episodes"))
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".md") && !strings.Contains(n, ".claims") {
			ids = append(ids, strings.TrimSuffix(n, ".md"))
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// NextEpisodeID gives "2026-09-02-a", "-b", ... for a recording date.
func (v *Vault) NextEpisodeID(recorded time.Time) (string, error) {
	day := recorded.Format("2006-01-02")
	ids, err := v.EpisodeIDs()
	if err != nil {
		return "", err
	}
	used := map[string]bool{}
	for _, id := range ids {
		if strings.HasPrefix(id, day+"-") {
			used[id] = true
		}
	}
	for c := 'a'; c <= 'z'; c++ {
		id := fmt.Sprintf("%s-%c", day, c)
		if !used[id] {
			return id, nil
		}
	}
	return "", fmt.Errorf("more than 26 entries on %s", day)
}

// Day is the day number: day 1 is the day of the earliest entry (including the one being added).
func (v *Vault) Day(recorded time.Time) (int, error) {
	ids, err := v.EpisodeIDs()
	if err != nil {
		return 0, err
	}
	first := recorded
	for _, id := range ids {
		if len(id) < 10 {
			continue
		}
		t, err := time.ParseInLocation("2006-01-02", id[:10], recorded.Location())
		if err == nil && t.Before(first) {
			first = t
		}
	}
	y1, m1, d1 := first.Date()
	y2, m2, d2 := recorded.Date()
	a := time.Date(y1, m1, d1, 0, 0, 0, 0, recorded.Location())
	b := time.Date(y2, m2, d2, 0, 0, 0, 0, recorded.Location())
	return int(b.Sub(a).Hours()/24) + 1, nil
}

func (v *Vault) AppendLog(line string) error {
	f, err := os.OpenFile(v.Path("_log.md"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if st, _ := f.Stat(); st != nil && st.Size() == 0 {
		if _, err := f.WriteString("# Log\n\nAppend-only. One line per operation; never edit earlier lines.\n\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(line + "\n")
	return err
}

// findTool resolves a program the way a person expects: the search path first, then the
// places package managers put things. A window opened from the Dock or at login carries
// only the bare system path, so Homebrew's folder would otherwise be invisible.
func findTool(name string) string {
	if strings.Contains(name, "/") {
		return name
	}
	// the copy inside the app first: it is the one that is always there
	if d := bundledToolsDir(); d != "" {
		if p := filepath.Join(d, name); fileThere(p) {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin",
		filepath.Join(home, ".local", "bin"), "/usr/bin", "/bin", "/snap/bin", "/usr/local/sbin"} {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return name
}

// Orphans lists recordings in raw/ that have no entry page: what a crash or a closed
// laptop leaves behind. Nothing is ever lost; it is processed later.
func (v *Vault) Orphans() ([]string, error) {
	clips, err := filepath.Glob(v.Path("raw", "*", "*", "*.mp4"))
	if err != nil {
		return nil, err
	}
	pages, _ := filepath.Glob(v.Path("episodes", "*.md"))
	var referenced strings.Builder
	for _, p := range pages {
		b, err := os.ReadFile(p)
		if err == nil {
			referenced.Write(b)
		}
	}
	all := referenced.String()
	var out []string
	for _, c := range clips {
		rel, _ := filepath.Rel(v.Root, c)
		if strings.Contains(all, rel) {
			continue
		}
		if _, err := os.Stat(strings.TrimSuffix(c, filepath.Ext(c)) + ".silent"); err == nil {
			continue // known to hold no words
		}
		out = append(out, c)
	}
	return out, nil
}

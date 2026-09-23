// Poiesis: talk to the camera, keep your own record, let any AI read it.
//
// Day-1 scope: `poiesis ingest` turns a video in the inbox into an entry page,
// a claims file and entity pages inside a plain-files vault; `poiesis lint`
// checks that every claim is backed by the transcript.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// dumpScreen renders one screen as plain text, for checking without a terminal.
func dumpScreen(m *tuiModel, name string) string {
	m.width, m.height = 100, 32
	switch name {
	case "entry":
		if len(m.data.entries) > 0 {
			m.openEntry(0)
			m.entry.earlier = true
		}
	case "entities":
		m.scr = screenEntities
	case "entity":
		if len(m.data.entities) > 0 {
			m.openEntity(m.data.entities[0].ID)
		}
	case "search":
		m.enterLog()
		m.log.search.SetValue("bakery")
		m.buildLogRows()
	case "log":
		m.enterLog()
	case "settings":
		m.enterSettings()
	case "ask":
		m.enterAsk()
	case "record":
		m.scr = screenRecord
	}
	return m.View() + "\n"
}

const version = "0.0.5"

func usage() {
	fmt.Fprintf(os.Stderr, `Poiesis %s

usage:
  poiesis                        open the app here in the terminal (record, entries, search)
  poiesis window [--record]      open the app in its own window (Ghostty, Alacritty, kitty, WezTerm; else Terminal)
  poiesis tray [--login]         menu bar / tray item: streak, Open, Record now; --login starts it when you log in
  poiesis ingest [file ...]      process every video in <vault>/inbox, or the given files
  poiesis ingest --orphans       process recordings in raw/ that never became entries (after a crash)
  poiesis lint                   check every claim against its transcript and the entity pages
  poiesis mcp                    serve the vault to an AI over stdio: orient, search, read, moment (read-only)
  poiesis mcp --connect          print the setup lines for Claude Code, Claude Desktop and Cursor
  poiesis setup                  check tools, download the speech models (verified), set the vault and extractor
                             flags: --install  --swedish  --extractor ollama|claude  --mcp  --app
  poiesis ask "…"               the local AI answers a question from the log, with the moments
  poiesis schema                 print SCHEMA.md
  poiesis dmg [file]             make the Mac install file from this build (build the host first: hosts/mac/build.sh)

flags (all commands):
  --vault DIR      vault folder (default: $POIESIS_VAULT or ~/Documents/Poiesis Vault)
  --lang auto|sv|en  language setting for this run (default: vault config, then auto)
  --extractor claude|ollama|file   who extracts claims (default: vault config, then claude)
  --model NAME     model for the extractor (default: claude-opus-5 / qwen3:8b)
  --claims-file F  with --extractor file: read the extraction result from this JSON file
  --mission NAME   mission label for the entries ingested in this run
  --keep-inbox     copy instead of move the inbox file into raw/
`, version)
}

func main() {
	// `poiesis` alone (or with only flags) opens the terminal app
	cmd := "ui"
	rest := os.Args[1:]
	if len(os.Args) >= 2 && !strings.HasPrefix(os.Args[1], "-") {
		cmd = os.Args[1]
		rest = os.Args[2:]
	}
	// started by double-click: the helper holds the menu bar item, the app opens the window
	if len(os.Args) == 1 && inMacApp() {
		if self, _ := os.Executable(); strings.Contains(self, "Poiesis Menu.app") {
			cmd = "tray"
		} else {
			cmd = "window"
		}
	}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	dump := fs.String("dump", "", "print one screen as text and exit: log, entry, entities, search, record, settings")
	connect := fs.Bool("connect", false, "with mcp: print the setup lines for Claude Code, Claude Desktop and Cursor")
	vaultDir := fs.String("vault", defaultVaultDir(), "vault folder")
	lang := fs.String("lang", "", "language setting: auto, sv, en")
	extractor := fs.String("extractor", "", "claude, ollama or file")
	model := fs.String("model", "", "model name for the extractor")
	claimsFile := fs.String("claims-file", "", "extraction result JSON for --extractor file")
	mission := fs.String("mission", "", "mission label")
	keepInbox := fs.Bool("keep-inbox", false, "copy instead of move")
	orphans := fs.Bool("orphans", false, "with ingest: process recordings in raw/ that never became entries")
	setupInstall := fs.Bool("install", false, "with setup: run the package manager and ollama pull")
	setupSwedish := fs.Bool("swedish", false, "with setup: also download the Swedish-only speech model")
	setupMCP := fs.Bool("mcp", false, "with setup: let Claude Code read this vault")
	setupApp := fs.Bool("app", false, "with setup: make ~/Applications/Poiesis.app (macOS)")
	startRecord := fs.Bool("record", false, "with ui/window: open on the record screen (the default)")
	startLog := fs.Bool("log", false, "with ui: open on the log page instead of the record screen")
	trayLogin := fs.Bool("login", false, "with tray: start at login")
	fs.Usage = usage
	if err := fs.Parse(rest); err != nil {
		os.Exit(2)
	}

	if r := bundleVault(); r != "" && *vaultDir == defaultVaultDir() {
		*vaultDir = r // the vault the bundle was set up with
	}
	if cmd == "window" {
		// do not touch the vault here: on a Mac the app that touches it is asked for
		// permission, and that should be the terminal window, not the launcher
		what := ""
		if *startRecord {
			what = "record"
		}
		if err := openOrFocus(*vaultDir, what); err != nil {
			fail(err)
		}
		return
	}
	if cmd == "tray" {
		if err := runTray(*vaultDir, *trayLogin); err != nil {
			fail(err)
		}
		return
	}
	if cmd == "dmg" { // made from the build alone: no vault is opened
		out := fs.Arg(0)
		if out == "" {
			out = "Poiesis-" + version + ".dmg"
		}
		if err := makeDMG(out); err != nil {
			fail(err)
		}
		fmt.Println(out)
		return
	}
	v, err := OpenVault(*vaultDir)
	if err != nil {
		fail(err)
	}
	if *lang != "" {
		v.Config.Language = *lang
	}
	if *extractor != "" {
		v.Config.Extractor = *extractor
	}
	if *model != "" {
		v.Config.Model = *model
	}

	switch cmd {
	case "ui", "view":
		if cmd == "ui" {
			theHost = connectHost() // nil unless a host started this core (docs/HOST.md)
		}
		m, err := newTUI(v)
		if err != nil {
			fail(err)
		}
		m.startOnLog = *startLog
		if *dump != "" {
			fmt.Print(dumpScreen(m, *dump))
			return
		}
		if _, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithReportFocus()).Run(); err != nil {
			fail(err)
		}
	case "setup":
		if err := runSetup(v, setupOptions{Install: *setupInstall, Swedish: *setupSwedish, Extractor: *extractor, MCP: *setupMCP, App: *setupApp}); err != nil {
			fail(err)
		}
	case "ingest":
		files := fs.Args()
		if *orphans {
			files, err = v.Orphans()
			if err != nil {
				fail(err)
			}
			if len(files) == 0 {
				fmt.Println("every recording has its entry")
				return
			}
			fmt.Printf("%d recording(s) never became entries; processing them now\n", len(files))
			*keepInbox = true
		}
		if len(files) == 0 {
			files, err = v.InboxFiles()
			if err != nil {
				fail(err)
			}
			if len(files) == 0 {
				fmt.Printf("inbox is empty: %s\n", v.Path("inbox"))
				return
			}
		}
		for _, f := range files {
			start := time.Now()
			ep, err := Ingest(context.Background(), v, IngestOptions{
				File: f, Mission: *mission, ClaimsFile: *claimsFile, KeepInbox: *keepInbox,
			})
			if errors.Is(err, ErrNoSpeech) {
				fmt.Printf("· %s  nothing was said; kept, no entry\n", filepath.Base(f))
				continue
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "✗ %s: %v\n", filepath.Base(f), err)
				continue
			}
			fmt.Printf("✓ %s  day %d  %d claims  %d entities  %.0fs\n", ep.ID, ep.Day, ep.ClaimCount, len(ep.Entities), time.Since(start).Seconds())
		}
	case "mcp":
		if *connect {
			fmt.Print(connectText(v))
			return
		}
		if err := runMCP(v); err != nil {
			fail(err)
		}
	case "lint":
		problems, err := Lint(v)
		if err != nil {
			fail(err)
		}
		if len(problems) == 0 {
			fmt.Println("lint: clean")
			return
		}
		for _, p := range problems {
			fmt.Println("lint:", p)
		}
		os.Exit(1)
	case "ask":
		q := strings.Join(fs.Args(), " ")
		if q == "" {
			fmt.Fprintln(os.Stderr, "poiesis ask \"what did I say about the bakery?\"")
			os.Exit(2)
		}
		d, err := loadTUIData(v)
		if err != nil {
			fail(err)
		}
		a, err := askLocal(v, q, askContext(d, q))
		if err != nil {
			fail(err)
		}
		fmt.Println(a)
		return
	case "version":
		fmt.Println("poiesis " + version)
		return
	case "schema":
		fmt.Print(schemaMD)
	default:
		usage()
		os.Exit(2)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "poiesis:", err)
	if inMacApp() {
		// opened from Poiesis.app: hold the window open so the message can be read
		fmt.Fprintln(os.Stderr, "\npress enter to close")
		fmt.Fscanln(os.Stdin)
	}
	os.Exit(1)
}

func defaultVaultDir() string {
	if d := os.Getenv("POIESIS_VAULT"); d != "" {
		return d
	}
	if b, err := os.ReadFile(filepath.Join(appStateDir(), "vault.txt")); err == nil {
		if p := strings.TrimSpace(string(b)); p != "" {
			return p // chosen on the settings page
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Documents", "Poiesis Vault")
}

// humanize turns an entity id into a display name: "familjetapeter" -> "Familjetapeter".
func humanize(id string) string {
	parts := strings.Split(id, "-")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// inMacApp: are we the executable inside an app bundle?
func inMacApp() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	self, err := os.Executable()
	return err == nil && strings.Contains(self, ".app/Contents/MacOS/")
}

// bundleVault reads the vault path the setup wrote next to the bundled binary.
func bundleVault() string {
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(self)), "Resources", "vault.txt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// The door: which AI apps can read this log through Poiesis, and a way to close each one
// (PRINCIPLES, rule 2). An AI app keeps its connections in its own settings file. Poiesis reads
// those files, and closes a door by taking its own entry out of one, after keeping a copy of
// the file as it was.

type aiApp struct {
	name   string // shown to the person
	config string // the app's own settings file
	cli    bool   // Claude Code rewrites its file often: its own command changes it
}

// knownAIApps is where each AI app keeps its connections.
func knownAIApps() []aiApp {
	home, _ := os.UserHomeDir()
	apps := []aiApp{
		{name: "Claude Code", config: filepath.Join(home, ".claude.json"), cli: true},
		{name: "Cursor", config: filepath.Join(home, ".cursor", "mcp.json")},
	}
	switch runtime.GOOS {
	case "darwin":
		apps = append(apps, aiApp{name: "Claude Desktop", config: filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")})
	case "windows":
		apps = append(apps, aiApp{name: "Claude Desktop", config: filepath.Join(os.Getenv("APPDATA"), "Claude", "claude_desktop_config.json")})
	}
	return apps
}

// poiesisServers is the names under which a settings file connects Poiesis.
func poiesisServers(config string) ([]string, error) {
	b, err := os.ReadFile(config)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f struct {
		MCPServers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(config), err)
	}
	var names []string
	for name, s := range f.MCPServers {
		if name == "poiesis" || filepath.Base(s.Command) == "poiesis" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// connectedAIApps is the AI apps whose settings connect Poiesis. A file that cannot be read is
// passed over: the door list shows what it knows.
func connectedAIApps() []aiApp {
	var out []aiApp
	for _, a := range knownAIApps() {
		if names, err := poiesisServers(a.config); err == nil && len(names) > 0 {
			out = append(out, a)
		}
	}
	return out
}

// claudeCodeRemove asks Claude Code itself to forget Poiesis; a test puts a fake in its place.
var claudeCodeRemove = func() error {
	out, err := exec.Command("claude", "mcp", "remove", "poiesis", "-s", "user").CombinedOutput()
	if err != nil {
		return fmt.Errorf("claude mcp remove: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// closeDoor takes Poiesis out of an AI app's settings, keeping the file as it was beside it.
// The app lets go when it starts again.
func closeDoor(a aiApp) error {
	if a.cli {
		return claudeCodeRemove()
	}
	names, err := poiesisServers(a.config)
	if err != nil || len(names) == 0 {
		return err
	}
	if real, err := filepath.EvalSymlinks(a.config); err == nil {
		a.config = real // a linked settings file is changed where it lives, and the link stays
	}
	b, err := os.ReadFile(a.config)
	if err != nil {
		return err
	}
	fi, err := os.Stat(a.config)
	if err != nil {
		return err
	}
	if err := os.WriteFile(a.config+".before-poiesis-removed", b, fi.Mode().Perm()); err != nil {
		return err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(b, &doc); err != nil {
		return err
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(doc["mcpServers"], &servers); err != nil {
		return err
	}
	for _, n := range names {
		delete(servers, n)
	}
	if doc["mcpServers"], err = json.Marshal(servers); err != nil {
		return err
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(a.config, append(out, '\n'), fi.Mode().Perm())
}

// aiAppNames is the connected apps as one line for settings.
func aiAppNames(apps []aiApp) string {
	if len(apps) == 0 {
		return "none can read your log"
	}
	var names []string
	for _, a := range apps {
		names = append(names, a.name)
	}
	return strings.Join(names, ", ") + " can read your whole log"
}

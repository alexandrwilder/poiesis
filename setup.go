package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// `poiesis setup` makes a fresh machine ready, out loud: which tools are there, which model
// files get downloaded (checksum-verified, into the app's own folder, only what was asked
// for), where the vault is, who extracts claims, and optionally the AI connection and a
// Mac app icon. It never installs anything without --install, and it prints the exact
// command it would run.

type modelFile struct {
	Name string
	URL  string
	SHA  string
	Size int64
	Why  string
}

var speechModels = []modelFile{
	{"ggml-large-v3-turbo-q5_0.bin", "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo-q5_0.bin",
		"394221709cd5ad1f40c46e6031ca61bce88931e6e088c188294c6d5a55ffa7e2", 574041195, "speech to text, all languages"},
	{"ggml-silero-v5.1.2.bin", "https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v5.1.2.bin",
		"29940d98d42b91fbd05ce489f3ecf7c72f0a42f027e4875919a28fb4c04ea2cf", 885098, "finds where you speak"},
}

var swedishModel = modelFile{"kb-whisper-large-q5_0.bin", "https://huggingface.co/KBLab/kb-whisper-large/resolve/main/ggml-model-q5_0.bin",
	"6d2863812d7410322bb7d8647a5c7260761300fa946714c9ed66d22bb30bcb19", 1081140203, "Swedish-only speech to text (optional)"}

const setupOllamaModel = "qwen3.5:4b"

type setupOptions struct {
	Install   bool // run the package manager and ollama pull
	Swedish   bool // also fetch the Swedish-only model
	Extractor string
	MCP       bool // register with Claude Code
	App       bool // make ~/Applications/Poiesis.app (macOS)
}

func runSetup(v *Vault, o setupOptions) error {
	say := func(f string, a ...any) { fmt.Printf(f+"\n", a...) }
	say("poiesis setup  ·  vault %s", v.Root)
	say("")

	// 1. tools
	missing := checkTools(v, say)
	if len(missing) > 0 {
		cmd := installCommand(missing)
		if o.Install && cmd != "" {
			say("  installing: %s", cmd)
			if err := runShell(cmd); err != nil {
				return fmt.Errorf("installing tools: %w", err)
			}
			checkTools(v, say)
		} else if cmd != "" {
			say("  to install them:  %s   (or run: poiesis setup --install)", cmd)
		}
	}
	say("")

	// 2. models
	say("MODELS  ·  %s", v.Config.ModelsDir)
	models := speechModels
	if o.Swedish {
		models = append(models, swedishModel)
	}
	for _, mf := range models {
		if err := ensureModel(v.Config.ModelsDir, mf, say); err != nil {
			return err
		}
	}
	say("")

	// 3. who extracts claims
	if o.Extractor != "" {
		v.Config.Extractor = o.Extractor
	}
	say("EXTRACTOR  ·  %s", v.Config.Extractor)
	switch v.Config.Extractor {
	case "ollama":
		if err := checkOllama(v, o.Install, say); err != nil {
			say("  %s", err)
		}
	case "claude":
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			say("  ✗ ANTHROPIC_API_KEY is not set. Your own key, used only for the extraction call. Put it in your shell profile.")
		} else {
			say("  ✓ ANTHROPIC_API_KEY is set (your key, your account)")
		}
	default:
		say("  ✗ unknown extractor %q: use --extractor ollama (local) or --extractor claude (your key)", v.Config.Extractor)
	}
	if err := v.SaveConfig(); err != nil {
		return err
	}
	say("")

	// 4. AI connection
	if o.MCP {
		if err := registerMCP(v, say); err != nil {
			say("  ✗ %s", err)
		}
		say("")
	}

	// 5. Mac app
	if o.App && runtime.GOOS == "darwin" {
		p, err := writeMacApp(v)
		if err != nil {
			return err
		}
		say("APP  ·  %s  (open it from Launchpad or Spotlight; it opens Poiesis in its own window)", p)
		say("")
	}

	say("READY.  start with:  poiesis        (here in the terminal)")
	say("                or:  poiesis window  (its own clean window)")
	say("nothing leaves this computer unless you chose --extractor claude with your own key.")
	return nil
}

func checkTools(v *Vault, say func(string, ...any)) []string {
	say("TOOLS")
	var missing []string
	for _, t := range []struct{ name, bin, why string }{
		{"ffmpeg", v.Config.FFmpegBin, "records the camera"},
		{"ffprobe", v.Config.FFprobeBin, "reads video details"},
		{"whisper-cli", v.Config.WhisperBin, "speech to text (whisper.cpp)"},
	} {
		if p := findTool(t.bin); strings.Contains(p, "/") {
			say("  ✓ %-12s %s", t.name, p)
		} else {
			say("  ✗ %-12s missing  ·  %s", t.name, t.why)
			missing = append(missing, t.name)
		}
	}
	return missing
}

// installCommand is the package-manager line for this OS; empty when there is no manager.
func installCommand(missing []string) string {
	needFF := false
	needWhisper := false
	for _, m := range missing {
		if m == "whisper-cli" {
			needWhisper = true
		} else {
			needFF = true
		}
	}
	var pk []string
	switch runtime.GOOS {
	case "darwin":
		if needFF {
			pk = append(pk, "ffmpeg")
		}
		if needWhisper {
			pk = append(pk, "whisper-cpp")
		}
		return "brew install " + strings.Join(pk, " ")
	case "linux":
		if _, err := exec.LookPath("pacman"); err == nil {
			var parts []string
			if needFF {
				parts = append(parts, "sudo pacman -S --needed ffmpeg")
			}
			if needWhisper {
				parts = append(parts, "yay -S whisper.cpp")
			}
			return strings.Join(parts, " && ")
		}
		if _, err := exec.LookPath("apt-get"); err == nil {
			var parts []string
			if needFF {
				parts = append(parts, "sudo apt-get install -y ffmpeg")
			}
			if needWhisper {
				parts = append(parts, "build whisper.cpp from https://github.com/ggml-org/whisper.cpp (cmake) and put whisper-cli on your PATH")
			}
			return strings.Join(parts, " ; ")
		}
	case "windows":
		var parts []string
		if needFF {
			parts = append(parts, "winget install Gyan.FFmpeg")
		}
		if needWhisper {
			parts = append(parts, "download whisper-cli from https://github.com/ggml-org/whisper.cpp/releases and put it on your PATH")
		}
		return strings.Join(parts, " ; ")
	}
	return ""
}

func runShell(cmd string) error {
	c := exec.Command("/bin/sh", "-c", cmd)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// ensureModel downloads one model file unless it is already there with the right checksum.
func ensureModel(dir string, mf modelFile, say func(string, ...any)) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(dir, mf.Name)
	if st, err := os.Stat(dst); err == nil && st.Size() == mf.Size {
		if sum, _ := fileSHA256(dst); sum == mf.SHA {
			say("  ✓ %-32s %8s  ·  %s", mf.Name, humanMB(mf.Size), mf.Why)
			return nil
		}
		say("  ! %s is there but its checksum is wrong; downloading again", mf.Name)
	}
	say("  ↓ %-32s %8s  ·  %s", mf.Name, humanMB(mf.Size), mf.Why)
	tmp := dst + ".part"
	if err := download(mf.URL, tmp, mf.Size, say); err != nil {
		return fmt.Errorf("downloading %s: %w", mf.Name, err)
	}
	sum, err := fileSHA256(tmp)
	if err != nil {
		return err
	}
	if sum != mf.SHA {
		os.Remove(tmp)
		return fmt.Errorf("%s: checksum mismatch after download; not keeping it", mf.Name)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	say("  ✓ %-32s verified", mf.Name)
	return nil
}

func download(url, dst string, size int64, say func(string, ...any)) error {
	client := &http.Client{Timeout: 0}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, 1<<20)
	var done int64
	last := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			done += int64(n)
			if size > 0 {
				setProgress("downloading", int(done*100/size), 0, 0)
			}
			if time.Since(last) > 2*time.Second {
				fmt.Printf("\r    %d / %d MB", done>>20, size>>20)
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	fmt.Print("\r                              \r")
	return nil
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// checkOllama: is Ollama running, and is the small local model pulled?
func checkOllama(v *Vault, install bool, say func(string, ...any)) error {
	model := v.Config.Model
	if model == "" {
		model = setupOllamaModel
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(strings.TrimRight(v.Config.OllamaURL, "/") + "/api/tags")
	if err != nil {
		return fmt.Errorf("✗ Ollama is not running at %s  ·  install from https://ollama.com (open source), then: ollama pull %s", v.Config.OllamaURL, model)
	}
	defer resp.Body.Close()
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&tags)
	for _, m := range tags.Models {
		if m.Name == model {
			say("  ✓ Ollama is running and %s is pulled (local, nothing leaves the machine)", model)
			return nil
		}
	}
	if install {
		say("  pulling %s with ollama …", model)
		return runShell("ollama pull " + model)
	}
	return fmt.Errorf("✗ Ollama runs but %s is not pulled  ·  run: ollama pull %s   (or: poiesis setup --install)", model, model)
}

// registerMCP tells Claude Code about this vault (user scope), replacing an older entry.
func registerMCP(v *Vault, say func(string, ...any)) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("Claude Code is not installed; to connect another AI app run: poiesis mcp --connect")
	}
	_ = exec.Command("claude", "mcp", "remove", "poiesis", "-s", "user").Run()
	out, err := exec.Command("claude", "mcp", "add", "--scope", "user", "poiesis", "--", self, "mcp", "--vault", v.Root).CombinedOutput()
	if err != nil {
		return fmt.Errorf("claude mcp add: %s", strings.TrimSpace(string(out)))
	}
	say("AI  ·  ✓ Claude Code can read this vault (read-only: orient, search, read, moment)")
	return nil
}

// writeMacApp builds ~/Applications/Poiesis.app, the way macOS expects a small app with a
// menu bar item: the app itself opens the log window and quits, and a helper inside it
// (Contents/Library/LoginItems/Poiesis Menu.app) holds the menu bar item and starts at login.
// They are separate bundles on purpose: one app cannot both keep the menu bar item and be
// launched again by double-clicking, because macOS only re-activates what is running.
// Both carry a copy of the binary, so `poiesis setup --app` refreshes them after a rebuild.
func writeMacApp(v *Vault) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	home := os.Getenv("HOME")
	app := filepath.Join(home, "Applications", "Poiesis.app")
	menu := filepath.Join(app, "Contents", "Library", "LoginItems", "Poiesis Menu.app")
	icns := filepath.Join(filepath.Dir(self), "assets", "Poiesis.icns")

	for _, b := range []struct {
		root, id, extra string
	}{
		{app, "app.poiesis", ""},
		{menu, "app.poiesis.menu", "  <key>LSUIElement</key><true/>\n"},
	} {
		macos := filepath.Join(b.root, "Contents", "MacOS")
		res := filepath.Join(b.root, "Contents", "Resources")
		for _, d := range []string{macos, res} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				return "", err
			}
		}
		if err := copyInto(self, filepath.Join(macos, "poiesis"), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(res, "vault.txt"), []byte(v.Root+"\n"), 0o644); err != nil {
			return "", err
		}
		if _, err := os.Stat(icns); err == nil {
			_ = copyInto(icns, filepath.Join(res, "Poiesis.icns"), 0o644)
		}
		name := "Poiesis"
		if b.id != "app.poiesis" {
			name = "Poiesis Menu"
		}
		plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>` + name + `</string>
  <key>CFBundleDisplayName</key><string>` + name + `</string>
  <key>CFBundleIdentifier</key><string>` + b.id + `</string>
  <key>CFBundleVersion</key><string>` + version + "." + fmt.Sprint(time.Now().Unix()) + `</string>
  <key>CFBundleShortVersionString</key><string>` + version + `</string>
  <key>CFBundleExecutable</key><string>poiesis</string>
  <key>CFBundleIconFile</key><string>Poiesis</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>NSHighResolutionCapable</key><true/>
` + b.extra + `  <key>NSDocumentsFolderUsageDescription</key><string>Poiesis keeps your entries in a folder in Documents. It reads and writes only that folder.</string>
  <key>NSCameraUsageDescription</key><string>Poiesis records you when you press space. Nothing leaves this computer.</string>
  <key>NSMicrophoneUsageDescription</key><string>Poiesis records your voice when you press space. Nothing leaves this computer.</string>
</dict></plist>
`
		if err := os.WriteFile(filepath.Join(b.root, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
			return "", err
		}
	}
	// the window host, renamed: without this the menu bar says "Ghostty" and the camera
	// indicator names it too, which is confusing when the app is called Poiesis
	if err := embedTerminal(app); err != nil {
		return "", err
	}
	// the tools, so the app works on a Mac with nothing installed
	if err := bundleTools(app, func(f string, a ...any) { fmt.Printf(f+"\n", a...) }); err != nil {
		return "", fmt.Errorf("putting the tools inside the app: %w", err)
	}

	// an unsigned app has no identity macOS can remember, so every launch asks again:
	// sign both bundles ad-hoc (helper first, then the app around it) and drop the
	// "downloaded" mark. A custom Finder icon (below) counts as "detritus" to the signer,
	// so it is removed before signing and put back after.
	stripCustomIcon(app)
	stripCustomIcon(filepath.Join(app, "Contents", "Frameworks", terminalAppName))
	_ = exec.Command("xattr", "-cr", app).Run()
	signOrder := []string{menu, app}
	if t := embeddedTerminal(); fileThere(t) && !signedAs(t, "app.poiesis.terminal") {
		signOrder = []string{t, menu, app} // sign the terminal once; after that leave it alone
	}
	for _, b := range signOrder {
		id := "app.poiesis"
		args := []string{"--force", "--sign", "-", "--identifier", id, "--timestamp=none"}
		if b == filepath.Join(app, "Contents", "Frameworks", terminalAppName) { // the window host, not the app around it
			args = []string{"--force", "--deep", "--sign", "-", "--identifier", id + ".terminal", "--timestamp=none"}
			say := func(string, ...any) {}
			_ = say
			if ent := filepath.Join(os.TempDir(), "poiesis-terminal-entitlements.plist"); true {
				_ = exec.Command("codesign", "-d", "--entitlements", ent, "--xml", ghosttyApp()).Run()
				if fileThere(ent) {
					args = append(args, "--entitlements", ent)
				}
			}
		}
		if out, err := exec.Command("codesign", append(args, b)...).CombinedOutput(); err != nil {
			return "", fmt.Errorf("signing %s: %s", filepath.Base(b), strings.TrimSpace(string(out)))
		}
	}
	// the icon as a custom Finder icon too: Finder and the Dock read it straight from the
	// bundle, past macOS's icon cache, so a new mark shows without a logout
	if png := filepath.Join(filepath.Dir(self), "assets", "icon.png"); fileThere(png) {
		setCustomIcon(app, png)
		setCustomIcon(filepath.Join(app, "Contents", "Frameworks", terminalAppName), png)
	}
	_ = exec.Command("/usr/bin/touch", app).Run()
	_ = exec.Command("/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister", "-f", app).Run()
	return app, nil
}

// stripCustomIcon removes a custom Finder icon so the bundle can be signed.
func stripCustomIcon(bundle string) {
	_ = os.Remove(filepath.Join(bundle, "Icon\r"))
	_ = exec.Command("SetFile", "-a", "c", bundle).Run()
}

// setCustomIcon gives a bundle a custom Finder icon from a PNG, with the developer
// tools' resource commands. Quietly does nothing where they are missing.
func setCustomIcon(bundle, png string) {
	for _, t := range []string{"sips", "DeRez", "Rez", "SetFile"} {
		if _, err := exec.LookPath(t); err != nil {
			return
		}
	}
	tmp := filepath.Join(os.TempDir(), "poiesis-icon-src.png")
	if err := copyInto(png, tmp, 0o644); err != nil {
		return
	}
	defer os.Remove(tmp)
	if err := exec.Command("sips", "-i", tmp).Run(); err != nil {
		return
	}
	rsrc, err := exec.Command("DeRez", "-only", "icns", tmp).Output()
	if err != nil || len(rsrc) == 0 {
		return
	}
	rfile := filepath.Join(os.TempDir(), "poiesis-icon.rsrc")
	if err := os.WriteFile(rfile, rsrc, 0o644); err != nil {
		return
	}
	defer os.Remove(rfile)
	iconFile := filepath.Join(bundle, "Icon\r")
	_ = os.Remove(iconFile)
	if err := exec.Command("Rez", "-append", rfile, "-o", iconFile).Run(); err != nil {
		return
	}
	_ = exec.Command("SetFile", "-a", "C", bundle).Run()
	_ = exec.Command("SetFile", "-a", "V", iconFile).Run()
}

// macMenuBinary is the helper inside Poiesis.app that may hold a menu bar item.
func macMenuBinary() string {
	app := outerAppBundle()
	if app == "" {
		home, _ := os.UserHomeDir()
		app = filepath.Join(home, "Applications", "Poiesis.app")
	}
	return filepath.Join(app, "Contents", "Library", "LoginItems", "Poiesis Menu.app", "Contents", "MacOS", "poiesis")
}

func copyInto(src, dst string, mode os.FileMode) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	_ = os.Remove(dst) // never write into a running binary
	return os.WriteFile(dst, b, mode)
}

func humanMB(n int64) string {
	mb := float64(n) / (1 << 20)
	if mb < 10 {
		return fmt.Sprintf("%.1f MB", mb)
	}
	return fmt.Sprintf("%.0f MB", mb)
}

const terminalAppName = "Poiesis.app"

// embeddedTerminal is the window host inside Poiesis.app.
func embeddedTerminal() string {
	app := outerAppBundle()
	if app == "" {
		return ""
	}
	return filepath.Join(app, "Contents", "Frameworks", terminalAppName)
}

// outerAppBundle finds Poiesis.app: around the running binary when it is inside it (wherever
// the person put the app), else in ~/Applications or /Applications.
func outerAppBundle() string {
	if self, err := os.Executable(); err == nil {
		if i := strings.Index(self, "/Poiesis.app/"); i >= 0 {
			return self[:i+len("/Poiesis.app")]
		}
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{filepath.Join(home, "Applications", "Poiesis.app"), "/Applications/Poiesis.app"} {
		if fileThere(p) {
			return p
		}
	}
	return ""
}

func ghosttyApp() string {
	for _, p := range []string{"/Applications/Ghostty.app", filepath.Join(os.Getenv("HOME"), "Applications", "Ghostty.app")} {
		if fileThere(p) {
			return p
		}
	}
	return ""
}

func fileThere(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// embedTerminal copies the terminal that draws the window into Poiesis.app and renames it, so
// macOS shows Poiesis everywhere: the menu bar, the camera indicator, the permission prompts.
// The terminal is Ghostty, MIT licensed; its notice travels with the copy.
func embedTerminal(app string) error {
	self, _ := os.Executable()
	src := ghosttyApp()
	if src == "" {
		return nil // no terminal to embed: the app still runs in whatever is installed
	}
	dst := filepath.Join(app, "Contents", "Frameworks", terminalAppName)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// keep the copy we have when it is the same version: macOS remembers permissions by
	// the app's signature, and a fresh copy would be a stranger that has to ask again
	if fileThere(dst) && plistValue(filepath.Join(dst, "Contents", "Info.plist"), "CFBundleVersion") ==
		plistValue(filepath.Join(src, "Contents", "Info.plist"), "CFBundleVersion") &&
		plistValue(filepath.Join(dst, "Contents", "Info.plist"), "CFBundleIdentifier") == "app.poiesis.terminal" {
		return nil
	}
	_ = os.RemoveAll(dst)
	if out, err := exec.Command("ditto", src, dst).CombinedOutput(); err != nil {
		return fmt.Errorf("copying the terminal: %s: %w", strings.TrimSpace(string(out)), err)
	}
	plist := filepath.Join(dst, "Contents", "Info.plist")
	for _, kv := range [][2]string{
		{"CFBundleName", "Poiesis"},
		{"CFBundleDisplayName", "Poiesis"},
		{"CFBundleIdentifier", "app.poiesis.terminal"},
		{"CFBundleShortVersionString", version},
		{"CFBundleVersion", version + "." + fmt.Sprint(time.Now().Unix())},
		{"NSCameraUsageDescription", "Poiesis records you when you press space. The video stays on this computer."},
		{"NSMicrophoneUsageDescription", "Poiesis records your voice when you press space. The audio stays on this computer."},
		{"NSDocumentsFolderUsageDescription", "Poiesis keeps your entries in a folder in Documents. It reads and writes only that folder."},
		{"SUEnableAutomaticChecks", "false"},
		{"SUAutomaticallyUpdate", "false"},
		{"SUAllowsAutomaticUpdates", "false"},
	} {
		kind := "string"
		if kv[1] == "false" || kv[1] == "true" {
			kind = "bool"
		}
		if out, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Set :"+kv[0]+" "+kv[1], plist).CombinedOutput(); err != nil {
			if out2, err2 := exec.Command("/usr/libexec/PlistBuddy", "-c", "Add :"+kv[0]+" "+kind+" "+kv[1], plist).CombinedOutput(); err2 != nil {
				return fmt.Errorf("terminal Info.plist %s: %s %s", kv[0], strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
			}
		}
	}
	for _, k := range []string{"SUFeedURL", "SUUpdateDriver"} {
		_ = exec.Command("/usr/libexec/PlistBuddy", "-c", "Delete :"+k, plist).Run()
	}
	// the Dock shows the running window host: give it Poiesis's icon, not the terminal's
	if icns := filepath.Join(filepath.Dir(self), "assets", "Poiesis.icns"); fileThere(icns) {
		_ = copyInto(icns, filepath.Join(dst, "Contents", "Resources", "Poiesis.icns"), 0o644)
		_ = exec.Command("/usr/libexec/PlistBuddy", "-c", "Set :CFBundleIconFile Poiesis", plist).Run()
		_ = os.Remove(filepath.Join(dst, "Contents", "Resources", "Assets.car")) // the old icon catalogue would win otherwise
		_ = os.Remove(filepath.Join(dst, "Contents", "Resources", "Ghostty.icns"))
		_ = exec.Command("/usr/libexec/PlistBuddy", "-c", "Delete :CFBundleIconName", plist).Run()
	}
	// the terminal's Dock tile plug-in is not needed (Poiesis sets its icon itself) and makes
	// macOS announce "a Dock tile plug-in was added"
	_ = os.RemoveAll(filepath.Join(dst, "Contents", "PlugIns"))
	if png := filepath.Join(filepath.Dir(self), "assets", "icon.png"); fileThere(png) {
		_ = copyInto(png, filepath.Join(app, "Contents", "Resources", "icon.png"), 0o644)
	}
	notice := `Poiesis draws its window with Ghostty, a terminal by Mitchell Hashimoto,
used here under the MIT license and renamed so that macOS shows this app's
own name in the menu bar and in the camera indicator. Source and license:
https://github.com/ghostty-org/ghostty

MIT License

Copyright (c) Mitchell Hashimoto

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
`
	return os.WriteFile(filepath.Join(app, "Contents", "Resources", "NOTICE-ghostty.txt"), []byte(notice), 0o644)
}

func plistValue(plist, key string) string {
	out, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print :"+key, plist).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// signedAs reports whether a bundle already carries our signature identifier.
func signedAs(bundle, id string) bool {
	out, _ := exec.Command("codesign", "-dv", bundle).CombinedOutput()
	return strings.Contains(string(out), "Identifier="+id)
}

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The tools inside the app: ffmpeg, ffprobe and whisper-cli with every library they need,
// copied into Poiesis.app and rewired to find each other there, so the other Mac needs
// nothing installed. findTool looks here first.

var bundledToolNames = []string{"ffmpeg", "ffprobe", "whisper-cli", "ollama"}

// bundledToolsDir is where the tools live inside the app, or "" outside the app.
func bundledToolsDir() string {
	app := outerAppBundle()
	if app == "" {
		return ""
	}
	return filepath.Join(app, "Contents", "Frameworks", "tools")
}

// bundleTools copies the three tools and their libraries into the app.
func bundleTools(app string, say func(string, ...any)) error {
	tools := filepath.Join(app, "Contents", "Frameworks", "tools")
	libs := filepath.Join(app, "Contents", "Frameworks", "lib")
	for _, d := range []string{tools, libs} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	copied := map[string]string{} // original path → bundled path
	var queue []string
	for _, name := range bundledToolNames {
		src := systemTool(name)
		if !strings.Contains(src, "/") { // an app without one of its tools must never be made
			return fmt.Errorf("%s is not installed here, so it cannot go into the app", name)
		}
		src, _ = filepath.EvalSymlinks(src)
		dst := filepath.Join(tools, name)
		if err := copyInto(src, dst, 0o755); err != nil {
			return err
		}
		copied[src] = dst
		queue = append(queue, dst)
	}
	// ggml loads its backends (Metal, CPU, BLAS) at run time by name, from the folder of
	// its own library: they never show as link-time references, so they are added by hand
	for _, name := range bundledToolNames {
		if name != "whisper-cli" {
			continue
		}
		src, _ := filepath.EvalSymlinks(systemTool(name))
		for _, dep := range foreignLibs(src) {
			real, err := filepath.EvalSymlinks(resolveRPath(src, dep))
			if err != nil || !strings.Contains(filepath.Base(real), "libggml") {
				continue
			}
			// the backends are .so files (Metal, the CPU variants, BLAS). ggml looks for them
			// next to the PROGRAM first, then in a folder compiled into the library (Homebrew's
			// own, which must never win: it would pull a second ggml runtime into the process
			// and crash). So they go beside whisper-cli, rewired to the app's own library.
			var backends []string
			for _, pat := range []string{"libggml-*.dylib", "libggml-*.so", "ggml/libggml-*.so", "../libexec/libggml-*.so", "../libexec/libggml-*.dylib"} {
				found, _ := filepath.Glob(filepath.Join(filepath.Dir(real), pat))
				backends = append(backends, found...)
			}
			for _, b := range backends {
				rb, err := filepath.EvalSymlinks(b)
				if err != nil {
					continue
				}
				base := filepath.Base(rb)
				if strings.HasPrefix(base, "libggml-base") || strings.HasPrefix(base, "libggml.") {
					continue // the core library itself is not a backend; it lives in lib/
				}
				if _, done := copied[rb]; done {
					continue
				}
				dst := filepath.Join(tools, filepath.Base(rb))
				if err := copyInto(rb, dst, 0o755); err != nil {
					return err
				}
				copied[rb] = dst
				_ = exec.Command("install_name_tool", "-id", "@loader_path/"+filepath.Base(rb), dst).Run()
				queue = append(queue, dst)
			}
			break
		}
	}
	// walk the libraries each file needs, copy them once, and rewire every reference
	for len(queue) > 0 {
		file := queue[0]
		queue = queue[1:]
		for _, dep := range foreignLibs(file) {
			real, err := filepath.EvalSymlinks(resolveRPath(file, dep))
			if err != nil {
				return fmt.Errorf("%s needs %s, which is missing", filepath.Base(file), dep)
			}
			dst, done := copied[real]
			if !done {
				dst = filepath.Join(libs, filepath.Base(real))
				if err := copyInto(real, dst, 0o755); err != nil {
					return err
				}
				copied[real] = dst
				if out, err := exec.Command("install_name_tool", "-id", "@loader_path/"+filepath.Base(real), dst).CombinedOutput(); err != nil {
					return fmt.Errorf("naming %s: %s", filepath.Base(real), strings.TrimSpace(string(out)))
				}
				queue = append(queue, dst)
			}
			ref := "@loader_path/" + filepath.Base(real)
			if filepath.Dir(file) == tools {
				ref = "@loader_path/../lib/" + filepath.Base(real)
			}
			if out, err := exec.Command("install_name_tool", "-change", dep, ref, file).CombinedOutput(); err != nil {
				return fmt.Errorf("rewiring %s: %s", filepath.Base(file), strings.TrimSpace(string(out)))
			}
		}
	}
	// ggml's library remembers the folder it was built to find its backends in (Homebrew's
	// own). Inside the app that memory must go, or on a Mac that has Homebrew the plug-ins
	// there would win over ours and drag a second runtime into the process. The path is
	// overwritten in place with a folder that does not exist, keeping the byte layout.
	for _, dst := range copied {
		if strings.HasPrefix(filepath.Base(dst), "libggml") {
			if err := forgetBuildPath(dst, "/libexec"); err != nil {
				return fmt.Errorf("editing %s: %w", filepath.Base(dst), err)
			}
		}
	}
	// a changed binary must be signed again on Apple silicon
	for _, dst := range copied {
		if out, err := exec.Command("codesign", "--force", "--sign", signIdentity(), dst).CombinedOutput(); err != nil {
			return fmt.Errorf("signing %s: %s", filepath.Base(dst), strings.TrimSpace(string(out)))
		}
	}
	if err := bundleLicences(app, copied, say); err != nil {
		return fmt.Errorf("the licence texts: %w", err)
	}
	say("  ✓ tools inside the app: %d programs, %d libraries", len(bundledToolNames), len(copied)-len(bundledToolNames))
	return nil
}

// bundleLicences puts the licence texts of every program and library inside the app into
// Contents/Resources/licenses, one folder per package named with its version (ffmpeg-9.0.2):
// whoever gets the app gets the licences with it.
func bundleLicences(app string, copied map[string]string, say func(string, ...any)) error {
	kegs := map[string]bool{} // a package's own folder: <prefix>/Cellar/<name>/<version>
	for src := range copied {
		i := strings.Index(src, "/Cellar/")
		if i < 0 {
			continue
		}
		if p := strings.SplitN(src[i+len("/Cellar/"):], "/", 3); len(p) >= 2 {
			kegs[src[:i]+"/Cellar/"+p[0]+"/"+p[1]] = true
		}
	}
	dir := filepath.Join(app, "Contents", "Resources", "licenses")
	for keg := range kegs {
		name := filepath.Base(filepath.Dir(keg)) + "-" + filepath.Base(keg)
		files, _ := filepath.Glob(filepath.Join(keg, "*"))
		found := 0
		for _, f := range files {
			up := strings.ToUpper(filepath.Base(f))
			if !strings.HasPrefix(up, "LICENSE") && !strings.HasPrefix(up, "LICENCE") && !strings.HasPrefix(up, "COPYING") && !strings.HasPrefix(up, "NOTICE") {
				continue
			}
			if fi, err := os.Stat(f); err != nil || fi.IsDir() {
				continue
			}
			if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
				return err
			}
			if err := copyInto(f, filepath.Join(dir, name, filepath.Base(f)), 0o644); err != nil {
				return err
			}
			found++
		}
		if found == 0 {
			say("  · no licence text found for %s: add it by hand before a release", name)
		}
	}
	return nil
}

// foreignLibs lists the libraries a file loads from a package manager's folders.
func foreignLibs(file string) []string {
	out, err := exec.Command("otool", "-L", file).Output()
	if err != nil {
		return nil
	}
	var deps []string
	for i, line := range strings.Split(string(out), "\n") {
		if i == 0 {
			continue
		}
		line = strings.TrimSpace(line)
		if j := strings.Index(line, " ("); j > 0 {
			line = line[:j]
		}
		if strings.HasPrefix(line, "/opt/homebrew") || strings.HasPrefix(line, "/usr/local") || strings.HasPrefix(line, "/opt/local") ||
			strings.HasPrefix(line, "@rpath/") {
			deps = append(deps, line)
		}
	}
	return deps
}

// resolveRPath turns "@rpath/lib.dylib" into a real path using the file's own search
// paths (LC_RPATH), the way the loader would; other references pass through.
func resolveRPath(file, dep string) string {
	if !strings.HasPrefix(dep, "@rpath/") {
		return dep
	}
	name := strings.TrimPrefix(dep, "@rpath/")
	out, err := exec.Command("otool", "-l", file).Output()
	if err != nil {
		return dep
	}
	lines := strings.Split(string(out), "\n")
	for i, l := range lines {
		if strings.Contains(l, "cmd LC_RPATH") {
			for j := i; j < i+4 && j < len(lines); j++ {
				t := strings.TrimSpace(lines[j])
				if strings.HasPrefix(t, "path ") {
					dir := strings.Fields(strings.TrimPrefix(t, "path "))[0]
					dir = strings.ReplaceAll(dir, "@loader_path", filepath.Dir(file))
					dir = strings.ReplaceAll(dir, "@executable_path", filepath.Dir(file))
					if fileThere(filepath.Join(dir, name)) {
						return filepath.Join(dir, name)
					}
				}
			}
		}
	}
	// last resort: the package manager's library folders
	for _, dir := range []string{"/opt/homebrew/lib", "/usr/local/lib", "/opt/local/lib"} {
		if fileThere(filepath.Join(dir, name)) {
			return filepath.Join(dir, name)
		}
	}
	return dep
}

// systemTool finds a program outside the app: the search path, then the package managers.
func systemTool(name string) string {
	if p, err := exec.LookPath(name); err == nil && !strings.Contains(p, "/Poiesis.app/") {
		return p
	}
	for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin", "/opt/homebrew/opt/ollama/bin"} {
		if p := filepath.Join(dir, name); fileThere(p) {
			return p
		}
	}
	return name
}

// forgetBuildPath blanks every C string in a binary that starts with "/" and ends with the
// given suffix, replacing it with "/nonexistent" padded with zero bytes to the same length.
func forgetBuildPath(file, suffix string) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	changed := false
	for i := 0; i < len(b); i++ {
		if b[i] != '/' || (i > 0 && b[i-1] != 0) {
			continue
		}
		end := i
		for end < len(b) && b[end] != 0 {
			end++
		}
		str := string(b[i:end])
		if strings.HasSuffix(str, suffix) && !strings.Contains(str, "\n") && len(str) >= len("/nonexistent") {
			repl := []byte("/nonexistent")
			copy(b[i:end], make([]byte, end-i))
			copy(b[i:], repl)
			changed = true
		}
		i = end
	}
	if !changed {
		return nil
	}
	return os.WriteFile(file, b, 0o755)
}

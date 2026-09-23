package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Updates come from the same place as installs: the newest release of the project on GitHub.
// install.sh, the install file and this code all use its files: Poiesis.dmg for the Mac app,
// poiesis_<os>_<arch>.tar.gz for the command on its own, checksums.txt for both. The check
// asks for the newest version number and sends nothing about the person. How a copy is
// updated follows how it was installed.

const releaseRepo = "alexandrwilder/poiesis"

// releasesURL is the project's release page; POIESIS_RELEASES points a test at another.
func releasesURL() string {
	if u := os.Getenv("POIESIS_RELEASES"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "https://github.com/" + releaseRepo + "/releases"
}

// latestVersion asks which version is the newest. GitHub answers /releases/latest with a
// redirect to /releases/tag/v<version>, and that is all that is read.
func latestVersion() (string, error) {
	c := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := c.Head(releasesURL() + "/latest")
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	i := strings.LastIndex(loc, "/tag/")
	if resp.StatusCode/100 != 3 || i < 0 {
		return "", fmt.Errorf("no release to be found at %s (%s)", releasesURL(), resp.Status)
	}
	return strings.TrimPrefix(loc[i+len("/tag/"):], "v"), nil
}

// newerVersion says whether a comes after b: the numbers part by part ("0.0.10" after
// "0.0.9"), and a plain number after the same number with a suffix ("0.0.5" after "0.0.5-day4").
func newerVersion(a, b string) bool {
	an, as := splitVersion(a)
	bn, bs := splitVersion(b)
	for i := 0; i < max(len(an), len(bn)); i++ {
		x, y := 0, 0
		if i < len(an) {
			x = an[i]
		}
		if i < len(bn) {
			y = bn[i]
		}
		if x != y {
			return x > y
		}
	}
	return as == "" && bs != ""
}

func splitVersion(v string) ([]int, string) {
	core, suffix, _ := strings.Cut(strings.TrimPrefix(v, "v"), "-")
	var nums []int
	for _, p := range strings.Split(core, ".") {
		n, _ := strconv.Atoi(p)
		nums = append(nums, n)
	}
	return nums, suffix
}

type installWay int

const (
	byMacApp         installWay = iota // Poiesis.app, from the install file or install.sh
	byHomeBinary                       // the command on its own in a folder of the person's
	byHomebrew                         // brew upgrade does it
	byPackageManager                   // the system's package manager does it
	bySource                           // built from the source here
)

// howInstalled says how the copy at exe came to be, and what an update replaces: the app
// around it, or the file itself.
func howInstalled(exe string) (installWay, string) {
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real // install.sh links the command in ~/.local/bin into the app
	}
	if i := strings.Index(exe, ".app/Contents/"); i >= 0 {
		return byMacApp, exe[:i+len(".app")]
	}
	switch {
	case strings.Contains(exe, "/Cellar/") || strings.HasPrefix(exe, "/opt/homebrew/") || strings.Contains(exe, "/linuxbrew/"):
		return byHomebrew, exe
	case strings.HasPrefix(exe, "/usr/bin/") || strings.HasPrefix(exe, "/bin/") || strings.HasPrefix(exe, "/usr/sbin/"):
		return byPackageManager, exe
	case fileThere(filepath.Join(filepath.Dir(exe), "go.mod")):
		return bySource, exe
	}
	return byHomeBinary, exe
}

// runUpdate is `poiesis update`: find the newest version and install it the way this copy
// was installed. It reports what it did in plain words.
func runUpdate(say func(string, ...any)) error {
	latest, err := latestVersion()
	if err != nil {
		return err
	}
	if !newerVersion(latest, version) {
		say("Poiesis %s is the newest", version)
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	way, target := howInstalled(exe)
	switch way {
	case byHomebrew:
		say("Poiesis %s is out. It came with Homebrew: brew upgrade poiesis", latest)
	case byPackageManager:
		say("Poiesis %s is out. Your system's package manager installed it; update it there", latest)
	case bySource:
		say("Poiesis %s is out. This copy is built from the source: git pull, then go build", latest)
	case byMacApp:
		if err := updateMacApp(target, latest, say); err != nil {
			return err
		}
		say("Poiesis %s is installed in %s. Open Poiesis again to use it.", latest, target)
	case byHomeBinary:
		if err := updateBinary(target, latest, say); err != nil {
			return err
		}
		say("Poiesis %s is installed in %s", latest, target)
	}
	return nil
}

// fetchVerified downloads one file of a release to dst and checks it against the release's
// checksums.txt; a file that does not match is removed and nothing else is touched.
func fetchVerified(ver, asset, dst string, say func(string, ...any)) error {
	base := releasesURL() + "/download/v" + ver + "/"
	sumsFile := dst + ".sums"
	defer os.Remove(sumsFile)
	if err := download(base+"checksums.txt", sumsFile, 0, say); err != nil {
		return fmt.Errorf("the release's checksums: %w", err)
	}
	sums, err := os.ReadFile(sumsFile)
	if err != nil {
		return err
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			want = f[0]
		}
	}
	if want == "" {
		return fmt.Errorf("the release has no %s for this system", asset)
	}
	say("downloading Poiesis %s", ver)
	if err := download(base+asset, dst, 0, say); err != nil {
		os.Remove(dst)
		return err
	}
	if got, err := fileSHA256(dst); err != nil || got != want {
		os.Remove(dst)
		return errors.New("the download does not match its checksum; nothing was changed")
	}
	return nil
}

// updateMacApp puts the new Poiesis.app where the old one is. The new app is copied from the
// install file beside the old one first, checked (whole, signed, the version asked for), and
// only then swapped in; if the swap fails, the old app goes back.
func updateMacApp(app, ver string, say func(string, ...any)) error {
	tmp, err := os.MkdirTemp("", "poiesis-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	dmg := filepath.Join(tmp, "Poiesis.dmg")
	if err := fetchVerified(ver, "Poiesis.dmg", dmg, say); err != nil {
		return err
	}
	mnt := filepath.Join(tmp, "mount")
	if out, err := exec.Command("hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mnt, dmg).CombinedOutput(); err != nil {
		return fmt.Errorf("opening the install file: %s", strings.TrimSpace(string(out)))
	}
	staged := filepath.Join(filepath.Dir(app), ".Poiesis-"+ver+".app") // the same disk, so the swap is one rename
	os.RemoveAll(staged)
	out, err := exec.Command("ditto", filepath.Join(mnt, "Poiesis.app"), staged).CombinedOutput()
	_ = exec.Command("hdiutil", "detach", mnt, "-quiet").Run()
	if err != nil {
		os.RemoveAll(staged)
		return fmt.Errorf("copying the new app: %s", strings.TrimSpace(string(out)))
	}
	if err := checkStagedApp(staged, ver); err != nil {
		os.RemoveAll(staged)
		return err
	}
	keepVaultChoice(app)
	previous := filepath.Join(filepath.Dir(app), ".Poiesis-previous.app")
	os.RemoveAll(previous)
	if err := os.Rename(app, previous); err != nil {
		os.RemoveAll(staged)
		return fmt.Errorf("could not set the old app aside: %w", err)
	}
	if err := os.Rename(staged, app); err != nil {
		_ = os.Rename(previous, app)
		return fmt.Errorf("could not put the new app in place; the old one is back: %w", err)
	}
	os.RemoveAll(previous)
	// the menu bar item runs from inside the app: start it again from the new one, when it
	// is this app's and not another copy's
	home, _ := os.UserHomeDir()
	if b, err := os.ReadFile(filepath.Join(home, "Library", "LaunchAgents", "app.poiesis.tray.plist")); err == nil && strings.Contains(string(b), app+"/") {
		_ = exec.Command("launchctl", "kickstart", "-k", "gui/"+uid()+"/app.poiesis.tray").Run()
	}
	return nil
}

// checkStagedApp refuses an app that is not whole, not signed as built, or not the version
// that was asked for.
func checkStagedApp(app, ver string) error {
	if out, err := exec.Command("codesign", "--verify", "--deep", "--strict", app).CombinedOutput(); err != nil {
		return fmt.Errorf("the new app's signature does not hold: %s", strings.TrimSpace(string(out)))
	}
	out, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print :CFBundleShortVersionString", filepath.Join(app, "Contents", "Info.plist")).Output()
	if err != nil || strings.TrimSpace(string(out)) != ver {
		return fmt.Errorf("the install file holds version %q, not %s", strings.TrimSpace(string(out)), ver)
	}
	return nil
}

// keepVaultChoice: an app set up for one vault carries its path inside; a new app from the
// install file does not, so the choice moves to the app's own folder, which updates keep.
func keepVaultChoice(app string) {
	pointer := filepath.Join(appStateDir(), "vault.txt")
	if fileThere(pointer) {
		return
	}
	if b, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", "vault.txt")); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		_ = os.MkdirAll(appStateDir(), 0o755)
		_ = os.WriteFile(pointer, b, 0o644)
	}
}

// updateBinary replaces the command at path with the one from the release, in one rename:
// a running copy keeps its old file until it ends.
func updateBinary(path, ver string, say func(string, ...any)) error {
	tmp, err := os.MkdirTemp("", "poiesis-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	tgz := filepath.Join(tmp, "poiesis.tar.gz")
	if err := fetchVerified(ver, fmt.Sprintf("poiesis_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH), tgz, say); err != nil {
		return err
	}
	fresh := filepath.Join(filepath.Dir(path), ".poiesis-"+ver)
	if err := extractOne(tgz, "poiesis", fresh); err != nil {
		os.Remove(fresh)
		return fmt.Errorf("unpacking the new version into %s: %w", filepath.Dir(path), err)
	}
	if err := os.Rename(fresh, path); err != nil {
		os.Remove(fresh)
		return fmt.Errorf("putting the new version in place: %w", err)
	}
	return nil
}

// extractOne writes the file called name from the .tar.gz at src to dst, as a program.
func extractOne(src, name, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("%s is not in the release file", name)
		}
		if err != nil {
			return err
		}
		if filepath.Base(h.Name) != name || h.Typeflag != tar.TypeReg {
			continue
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, tr)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		return err
	}
}

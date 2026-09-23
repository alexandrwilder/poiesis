package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewerVersion(t *testing.T) {
	for _, c := range []struct {
		a, b  string
		newer bool
	}{
		{"0.0.6", "0.0.5", true},
		{"0.0.10", "0.0.9", true},
		{"0.1.0", "0.0.99", true},
		{"0.0.5", "0.0.5-day4", true},
		{"v0.0.6", "0.0.5", true},
		{"0.0.5", "0.0.5", false},
		{"0.0.4", "0.0.5", false},
		{"0.0.5-day4", "0.0.5", false},
	} {
		if got := newerVersion(c.a, c.b); got != c.newer {
			t.Errorf("newerVersion(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

// Each way Poiesis can be installed is updated its own way: the app as a whole, the command
// on its own in place, and Homebrew, packages and source builds by their own tools.
func TestHowInstalled(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module poiesis\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		exe    string
		way    installWay
		target string
	}{
		{"/Applications/Poiesis.app/Contents/MacOS/poiesis", byMacApp, "/Applications/Poiesis.app"},
		{"/Applications/Poiesis.app/Contents/Library/LoginItems/Poiesis Menu.app/Contents/MacOS/poiesis", byMacApp, "/Applications/Poiesis.app"},
		{"/home/ada/.local/bin/poiesis", byHomeBinary, "/home/ada/.local/bin/poiesis"},
		{"/opt/homebrew/bin/poiesis", byHomebrew, "/opt/homebrew/bin/poiesis"},
		{"/usr/bin/poiesis", byPackageManager, "/usr/bin/poiesis"},
		{filepath.Join(src, "poiesis"), bySource, filepath.Join(src, "poiesis")},
	} {
		way, target := howInstalled(c.exe)
		if way != c.way || target != c.target {
			t.Errorf("%s: got way %d target %s, want %d %s", c.exe, way, target, c.way, c.target)
		}
	}
}

// fakeReleases serves the paths GitHub serves for a release of the given version, with the
// command on its own for this system and a checksums file; sums may be spoiled on purpose.
func fakeReleases(t *testing.T, ver string, program []byte, spoil bool) *httptest.Server {
	t.Helper()
	var tgz bytes.Buffer
	gz := gzip.NewWriter(&tgz)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "poiesis", Mode: 0o755, Size: int64(len(program)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	tw.Write(program)
	tw.Close()
	gz.Close()
	asset := fmt.Sprintf("poiesis_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(tgz.Bytes())
	if spoil {
		sum[0] ^= 0xff
	}
	sums := hex.EncodeToString(sum[:]) + "  " + asset + "\n"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			http.Redirect(w, r, "/releases/tag/v"+ver, http.StatusFound)
		case "/releases/download/v" + ver + "/checksums.txt":
			w.Write([]byte(sums))
		case "/releases/download/v" + ver + "/" + asset:
			w.Write(tgz.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestLatestVersion(t *testing.T) {
	srv := fakeReleases(t, "0.0.9", []byte("x"), false)
	defer srv.Close()
	t.Setenv("POIESIS_RELEASES", srv.URL+"/releases")
	got, err := latestVersion()
	if err != nil || got != "0.0.9" {
		t.Fatalf("latestVersion = %q, %v", got, err)
	}
	t.Setenv("POIESIS_RELEASES", srv.URL+"/nothing-here")
	if _, err := latestVersion(); err == nil {
		t.Fatal("a page without a release must say so, not pass")
	}
}

// The command on its own is replaced by the release's, and only when the download matches
// its checksum.
func TestUpdateBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "poiesis")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	quiet := func(string, ...any) {}

	spoiled := fakeReleases(t, "0.0.9", []byte("new"), true)
	t.Setenv("POIESIS_RELEASES", spoiled.URL+"/releases")
	if err := updateBinary(path, "0.0.9", quiet); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("a download that does not match must be refused, got %v", err)
	}
	spoiled.Close()
	if b, _ := os.ReadFile(path); string(b) != "old" {
		t.Fatalf("a refused update changed the command: %q", b)
	}

	good := fakeReleases(t, "0.0.9", []byte("new"), false)
	defer good.Close()
	t.Setenv("POIESIS_RELEASES", good.URL+"/releases")
	if err := updateBinary(path, "0.0.9", quiet); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	st, _ := os.Stat(path)
	if string(b) != "new" || st.Mode().Perm()&0o100 == 0 {
		t.Fatalf("after the update the command is %q with mode %v", b, st.Mode())
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".poiesis-*")); len(left) > 0 {
		t.Fatalf("the update left %v behind", left)
	}
}

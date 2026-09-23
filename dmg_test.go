package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The install file's app is for another Mac: it must never carry this Mac's vault path, and
// its signature must hold under the strict check macOS makes of a downloaded app.
func TestAppForAnotherMacCarriesNoVault(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the Mac app is made on a Mac")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if host, _ := macHost(self); host == "" {
		t.Skip("the Mac host is not built (hosts/mac/build.sh)")
	}
	app, err := writeMacApp(filepath.Join(t.TempDir(), "Poiesis.app"), "")
	if err != nil {
		t.Fatal(err)
	}
	_ = filepath.Walk(app, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.Name() == "vault.txt" || info.Name() == "Icon\r" {
			t.Errorf("the app for another Mac carries %s", strings.TrimPrefix(p, app))
		}
		// the read-me's one line takes the download mark off every file; it cannot on a
		// read-only one
		if info.Mode()&0o200 == 0 && info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s is read-only", strings.TrimPrefix(p, app))
		}
		return nil
	})
	if out, err := exec.Command("codesign", "--verify", "--deep", "--strict", app).CombinedOutput(); err != nil {
		t.Errorf("the signature does not hold: %s", out)
	}
	plist, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<string>PoiesisHost</string>", "<key>LSMinimumSystemVersion</key><string>14.0</string>"} {
		if !strings.Contains(string(plist), want) {
			t.Errorf("Info.plist lacks %s", want)
		}
	}
}

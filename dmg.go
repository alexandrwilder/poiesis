package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// dmgReadMe goes into the install file beside the app. %s is the version.
const dmgReadMe = `Poiesis %s, a preview for a few people

1. Drag Poiesis onto Applications.
2. Open Terminal (Applications > Utilities), paste this line and press return:

       xattr -dr com.apple.quarantine /Applications/Poiesis.app

   Poiesis is not signed by Apple yet. The line tells macOS that you chose to
   install it; without it macOS stops Poiesis and the programs inside it.
3. Open Poiesis. It asks once for the Documents folder, the camera and the
   microphone.

Your entries stay on this Mac, as plain files in Documents > Poiesis Vault.
The first entry fetches the speech model once (about 0.6 GB) and the local
AI's model once (about 3.4 GB). While Poiesis is open it asks GitHub every
hour for the newest version number; settings can turn that off. Your
recordings and words never leave this Mac, unless you add your own Claude key.

Needs a Mac with Apple silicon (M1 or later) and macOS 14 or later.
`

// makeDMG makes the Mac install file from this build: Poiesis.app built fresh for another
// Mac in a temporary folder, a link to Applications and the read-me, in one compressed disk
// image at out. It refuses without the Mac host, and when the app's signature does not hold.
func makeDMG(out string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("the Mac install file is made on a Mac")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if host, _ := macHost(self); host == "" {
		return errors.New("the Mac host is not built: run hosts/mac/build.sh first")
	}
	stage, err := os.MkdirTemp("", "poiesis-dmg-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	app, err := writeMacApp(filepath.Join(stage, "Poiesis.app"), "")
	if err != nil {
		return err
	}
	if o, err := exec.Command("codesign", "--verify", "--deep", "--strict", app).CombinedOutput(); err != nil {
		return fmt.Errorf("the app's signature does not hold: %s", strings.TrimSpace(string(o)))
	}
	if err := os.Symlink("/Applications", filepath.Join(stage, "Applications")); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, "READ ME FIRST.txt"), []byte(fmt.Sprintf(dmgReadMe, version)), 0o644); err != nil {
		return err
	}
	if o, err := exec.Command("hdiutil", "create", "-volname", "Poiesis", "-srcfolder", stage, "-format", "UDZO", "-ov", out).CombinedOutput(); err != nil {
		return fmt.Errorf("making the disk image: %s", strings.TrimSpace(string(o)))
	}
	return nil
}

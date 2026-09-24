package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// An entry cut short by a quit or a crash leaves its parts in inbox/.parts. The next start
// joins them into one clip to be processed; a part no program can read is kept aside.
func TestAnUnfinishedEntryIsJoinedAndKept(t *testing.T) {
	ffmpeg, ffprobe := findTool("ffmpeg"), findTool("ffprobe")
	if _, err := exec.LookPath(ffmpeg); err != nil {
		t.Skip("needs ffmpeg")
	}
	v, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parts := v.Path("inbox", ".parts")
	if err := os.MkdirAll(parts, 0o755); err != nil {
		t.Fatal(err)
	}
	// the first entry in two parts, a second entry in one, and a third that was never closed
	for _, name := range []string{"2026-09-24T10-00-00.part01.mp4", "2026-09-24T10-01-00.part02.mp4", "2026-09-24T10-03-00.part01.mp4"} {
		out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "testsrc=size=160x90:rate=10", "-f", "lavfi", "-i", "sine=frequency=440",
			"-t", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", filepath.Join(parts, name)).CombinedOutput()
		if err != nil {
			t.Skipf("this ffmpeg cannot make a test clip: %s", out)
		}
	}
	broken := "2026-09-24T10-05-00.part01.mp4"
	if err := os.WriteFile(filepath.Join(parts, broken), []byte("not a video"), 0o644); err != nil {
		t.Fatal(err)
	}

	waiting, err := v.waitingRecordings()
	if err != nil {
		t.Fatal(err)
	}
	joined, second := v.Path("inbox", "2026-09-24T10-00-00.mp4"), v.Path("inbox", "2026-09-24T10-03-00.mp4")
	if len(waiting) != 2 || waiting[0].file != joined || waiting[1].file != second || waiting[0].inRaw {
		t.Fatalf("waiting: %+v", waiting)
	}
	pr, err := probe(ffprobe, joined)
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := strconv.ParseFloat(pr.Format.Duration, 64); d < 1.8 {
		t.Fatalf("the joined clip is %.2f s; both parts should be in it", d)
	}
	if _, err := os.Stat(filepath.Join(parts, "unreadable", broken)); err != nil {
		t.Fatalf("the unreadable part was not kept: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(parts, "*.mp4")); len(left) != 0 {
		t.Fatalf("parts left behind: %v", left)
	}
}

// The window takes up what was waiting when it opens: processed one at a time, the picture
// resting meanwhile.
func TestTheWindowProcessesWhatWasWaiting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	opened := countCameraOpens(t)
	v, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v.Path("inbox", "2026-09-24T09-00-00.mp4"), []byte("a clip"), 0o644); err != nil {
		t.Fatal(err)
	}
	v.Config.Reflection = "on"
	m := &tuiModel{v: v, data: &tuiData{}, focused: true}
	m.Init()
	if m.record.pending != 1 || !m.record.processing {
		t.Fatalf("pending %d, processing %v", m.record.pending, m.record.processing)
	}
	if *opened != 0 {
		t.Fatalf("the picture started while a waiting entry is processed")
	}
}

// A recording whose camera stops on its own keeps what it has and says so, instead of a clock
// counting over nothing.
func TestARecordingThatStopsOnItsOwnSaysSo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	countCameraOpens(t)
	m := &tuiModel{v: &Vault{Config: defaultConfig()}, data: &tuiData{}}
	m.v.Config.Reflection = "off"
	cam := &fakeCamera{el: 40 * time.Second, end: make(chan error, 1)}
	m.scr, m.record.phase, m.record.cap = screenRecord, "recording", cam
	cam.end <- errors.New("the camera was unplugged")
	m.Update(tickMsg(time.Now()))
	if m.record.phase != "paused" || m.record.cap != nil || m.record.recorded != 40*time.Second {
		t.Fatalf("phase %q, camera %v, recorded %v", m.record.phase, m.record.cap, m.record.recorded)
	}
	if !strings.Contains(m.record.lastErr, "the recording stopped") {
		t.Fatalf("nothing was said: %q", m.record.lastErr)
	}
}

// With another Poiesis window open, its parts in inbox/.parts may belong to an entry it is
// still recording: this window leaves them alone.
func TestASecondWindowLeavesTheFirstWindowsPartsAlone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	countCameraOpens(t)
	v, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v.Path("inbox", "2026-09-24T09-00-00.mp4"), []byte("a clip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(appStateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	// the process that runs this test stands in for the other window: alive, and not us
	if err := os.WriteFile(windowPIDFile(), []byte(strconv.Itoa(os.Getppid())), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &tuiModel{v: v, data: &tuiData{}, focused: true}
	m.Init()
	if m.record.pending != 0 || m.record.processing {
		t.Fatalf("a second window took up the first one's work: pending %d", m.record.pending)
	}
}

// Two Poiesis programs started in the same moment never both take up what waits: the one
// that holds the lock does; a lock left by a crash is taken over after ten minutes.
func TestRecoveryRunsInOneProcessAtATime(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	v, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v.Path("inbox", "2026-09-24T09-00-00.mp4"), []byte("a clip"), 0o644); err != nil {
		t.Fatal(err)
	}
	release, ok := takeRecoveryLock() // the other program
	if !ok {
		t.Fatal("no lock to begin with")
	}
	m := &tuiModel{v: v, data: &tuiData{}}
	if m.processWaiting(); m.record.pending != 0 {
		t.Fatalf("both programs took up the waiting clip: pending %d", m.record.pending)
	}
	release()
	lock := filepath.Join(appStateDir(), "recover.lock")
	if err := os.WriteFile(lock, nil, 0o644); err != nil { // a crash left it
		t.Fatal(err)
	}
	old := time.Now().Add(-20 * time.Minute)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	if m.processWaiting(); m.record.pending != 1 {
		t.Fatalf("a lock left by a crash blocked recovery: pending %d", m.record.pending)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatal("the lock was not let go")
	}
}

// What the person should know about how an entry was saved reaches them, whatever the result.
func TestAnEntrysNoteReachesThePerson(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	countCameraOpens(t)
	m := &tuiModel{v: &Vault{Config: defaultConfig()}, data: &tuiData{}}
	m.record.phase, m.record.processing, m.record.pending = "ready", true, 2
	m.Update(ingestDoneMsg{err: errors.New("the model failed"), note: "1 part(s) could not be read"})
	if !strings.Contains(m.record.lastErr, "could not be read") || !strings.Contains(m.record.lastErr, "the model failed") {
		t.Fatalf("after a failure: %q", m.record.lastErr)
	}
	m.Update(ingestDoneMsg{err: ErrNoSpeech, note: "the camera closed late"})
	if !strings.Contains(m.status, "closed late") {
		t.Fatalf("after a silent clip: %q", m.status)
	}
}

// A part no program can read is kept aside and counted, so the person can be told.
func TestUnreadablePartsAreCounted(t *testing.T) {
	ffprobe := findTool("ffprobe")
	if _, err := exec.LookPath(ffprobe); err != nil {
		t.Skip("needs ffprobe")
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "2026-09-24T10-00-00.part01.mp4")
	if err := os.WriteFile(bad, []byte("not a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	readable, kept, err := readableParts(ffprobe, []string{bad})
	if err != nil || len(readable) != 0 || kept != 1 {
		t.Fatalf("readable %v, kept aside %d, %v", readable, kept, err)
	}
}

// A page is written whole or not at all, and nothing is left beside it.
func TestPagesAreWrittenWhole(t *testing.T) {
	dir := t.TempDir()
	page := filepath.Join(dir, "erik.md")
	for _, body := range []string{"the first version\n", "the second, longer version of the page\n"} {
		if err := writeFileAtomic(page, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(page); string(b) != body {
			t.Fatalf("the page holds %q", b)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".*")); len(left) != 0 {
		t.Fatalf("left beside the page: %v", left)
	}
}

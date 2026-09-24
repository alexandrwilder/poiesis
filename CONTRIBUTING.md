# Contributing to Poiesis

Thank you for looking. This file is the map: how to build it, where things are, where it
was made to be extended, and the few rules that keep it honest.

Looking for something to do? The issues marked
[good first issue](https://github.com/alexandrwilder/poiesis/labels/good%20first%20issue) need
no knowledge of the rest of the code, and the ones marked
[help wanted](https://github.com/alexandrwilder/poiesis/labels/help%20wanted) need a Linux or
Windows machine this project does not have.

## Build and run

    go build -o poiesis .            # one binary (Go 1.26)
    go test ./...                 # the tests; some need a camera or the network:
    POIESIS_CAPTURE_TEST=1 go test -run TestCapture .       # records 3 s from the camera
    POIESIS_NET_TEST=1 go test -run TestEnsureModel .       # downloads the small model
    ./poiesis --vault ~/Documents/"Poiesis Vault Test"        # run against a test vault

The window around it, per system (`docs/ARCHITECTURE.md`):

    hosts/mac/build.sh             # on a Mac: the window (Swift); it also builds the core
    ./poiesis setup --app          # then: assemble Poiesis.app with that window inside
    hosts/mac/test.sh              # the window's contract test (uses the camera)
    hosts/linux/test.sh            # the Linux window in Arch under Docker, on any machine

The core is Go; on a Mac the menu bar item and the window focus use a little Objective-C
through cgo, so Xcode's command line tools are needed there. A release is made by
`release.sh` on the maintainer's Mac.

## The map of the code

| area | files |
|---|---|
| the vault: folders, settings, ids, names | `vault.go`, `schema.go`, `ulid.go` |
| recording and the camera picture | `capture.go`, `reflection.go`, `look.go`, `kitty.go`, `winsize.go` |
| speech to text, and which language is spoken | `transcribe.go`, `language.go`, `progress.go` |
| claims: the extractor and the prompt | `extract.go` |
| writing the vault: entries, claims, entity pages, index | `write.go`, `entry_ops.go`, `media.go`, `lint.go` |
| the screens | `tui.go` (frame, keys), `tui_record.go`, `tui_log.go`, `tui_entry.go`, `tui_entities.go`, `tui_settings.go`, `tui_helpers.go`, `mouse.go`, `missionpick.go`, `theme.go` |
| the AI connection (MCP) | `mcp.go` |
| the camera seam and the window programs (`docs/HOST.md`) | `camera.go`, `host.go`, `hostcam.go`; `hosts/mac` (Swift), `hosts/linux` (C), `hosts/contract` (the test every one of them passes) |
| the Mac app, the window, the menu bar item | `window.go`, `setup.go`, `bundle_tools.go`, `tray*.go`, `focus_*.go`, `ollama.go`, `devices.go`, `detach*.go` |
| installing, updating and releasing | `install.sh`, `update.go`, `tui_update.go`, `dmg.go`, `release.sh` |
| the command line | `main.go` |

## Where it was made to be extended

- **Themes** — `theme.go`. One struct per theme: a few colours and a name. Add one, it
  appears in settings.
- **Looks** — `look.go`. One function per look: a pixel in, a pixel out. Add one, it
  appears in settings, with intensity for free.
- **Extractors** — `extract.go`, the `Extractor` interface: `Name`, `Extract`, `Release`.
  Two exist (local through Ollama, Claude through a key). A third is one file.
- **Missions** — a folder of pages in the vault. Anything that writes a mission page is
  a mission source.

## The rules

1. **Nothing about extraction ships without a score.** The maintainer keeps a gold set and a
   scorer and runs them on any change to the prompt or the model; the number goes in the
   review. A change is judged by the number, never by eye.
2. **Simple English, everywhere a person reads.** Screen text, errors, this file. No
   jargon; say what happens.
3. **The vault is the truth, the app is a view.** Videos are ground truth; pages and
   claims are rebuilt from them. Never write to the vault in a way another tool cannot
   read.
4. **Nothing leaves the computer** unless the person chose it, and then the settings page
   says so.
5. **Every coloured line goes through `cut()`,** never `fit()`; the second counts colour
   codes and breaks glyphs.
6. **Tests pin intent.** A test that cannot fail when the behaviour changes is not a test.
7. **Verifiers cost money.** CI runs the tests on a push and nothing else.

## The opinions, written down

The decided opinions are written down with their dates and reasons in the maintainer's notes.
Argue with a decision, not with a mystery: open an issue that names it, and ask for the note.

## Signing off

Every commit carries a sign-off (`git commit -s`). With it you certify the Developer
Certificate of Origin (developercertificate.org): you wrote the change, or have the right to
give it under this project's licences. You keep your copyright. The app stays AGPL-3.0 and the
format MIT.

## Voice

The app talks like a log entry, not like software: short, warm, exact. Copy that says
what happens beats copy that sells.

# Contributing to Poiesis

Thank you for looking. This file is the map: how to build it, where things are, where it
was made to be extended, and the few rules that keep it honest.

## Build and run

    go build -o poiesis .            # one binary
    go test ./...                 # the tests; some need a camera or the network:
    POIESIS_CAPTURE_TEST=1 go test -run TestCapture .       # records 3 s from the camera
    POIESIS_NET_TEST=1 go test -run TestEnsureModel .       # downloads the small model
    ./poiesis --vault ~/Documents/"Poiesis Vault Test"        # run against a test vault
    ./poiesis setup --app                                  # on a Mac: assemble Poiesis.app

Everything is Go; on a Mac the menu bar item and the window focus use a little
Objective-C through cgo, so Xcode's command line tools are needed there.

## The map of the code

| area | files |
|---|---|
| the vault: folders, settings, ids, names | `vault.go`, `schema.go`, `ulid.go` |
| recording and the camera picture | `capture.go`, `reflection.go`, `look.go`, `kitty.go`, `winsize.go` |
| speech to text | `transcribe.go`, `progress.go` |
| claims: the extractor and the prompt | `extract.go` |
| writing the vault: entries, claims, entity pages, index | `write.go`, `entry_ops.go`, `media.go`, `lint.go` |
| the screens | `tui.go` (frame, keys), `tui_record.go`, `tui_log.go`, `tui_entry.go`, `tui_entities.go`, `tui_settings.go`, `tui_helpers.go`, `mouse.go`, `missionpick.go`, `theme.go` |
| the AI connection (MCP) | `mcp.go` |
| the Mac app, the window, the menu bar item | `window.go`, `setup.go`, `bundle_tools.go`, `tray*.go`, `focus_*.go`, `ollama.go`, `devices.go` |
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

1. **Nothing about extraction ships without a score.** `plans/eval/` holds the gold set
   and the scorer. A prompt or model change is judged by the number, never by eye.
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

`../plans/calibration.md` is the sheet of decided opinions with their dates and reasons.
Argue with a decision, not with a mystery: open an issue that names the row.

## Voice

The app talks like a log entry, not like software: short, warm, exact. Copy that says
what happens beats copy that sells.

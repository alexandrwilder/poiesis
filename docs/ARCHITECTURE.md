# How Poiesis is built

The log is a folder of plain files. One program, the **core**, records into it, reads it,
shows it and lets an AI read it. On each system a small **host** can own the window, the
camera and the recording, and runs the core inside a text view. Between the two passes one
written contract, `HOST.md`. Everything below exists to keep that shape for a long time.

## Three layers, one direction

1. **The vault.** Files that outlive the software: markdown pages, claims as JSON lines,
   the recordings as mp4. `FORMAT.md` explains it; any tool can read it without Poiesis.
2. **The core.** One Go binary per system: the pipeline (recording, words, claims, pages),
   the text interface, the AI connection. It runs in any terminal on any system. It knows
   nothing about windows or camera APIs; it asks for a picture and a recording through one
   seam in the code, the camera, and whatever sits behind that seam answers.
3. **The host.** Optional, one per system, thin: a native window with the camera drawn by the
   system, the recording made by the system's encoder, and a text view running the core. It
   holds no log logic and never reads the vault.

Dependencies point one way: the host starts the core, the core writes the vault. The core
never needs a host. Replacing a host changes no entry.

## The contract

`HOST.md` is the whole agreement between core and host: small messages, one JSON object per
line, over a local socket. Three rules keep it working as both sides change:

- **Ask, never assume.** The host says what it can do when they meet; the core uses only that.
- **Add, never change.** Unknown messages and fields are ignored on both sides. A field is
  never renamed or given a new meaning; a change that breaks something is a new version, and
  both sides say which versions they speak.
- **Plain data only,** so a host can be written in any language.

## Ladders: the best path this machine has

Each capability has a ladder. At start the core takes the highest step that is there, shows
which one in settings, and steps down on its own when the machine struggles.

| what | ladder, best first |
|---|---|
| the picture | drawn by the host → terminal graphics (the kitty protocol; sixel planned, for Foot and Windows Terminal) → a picture in characters → off |
| the recording | made by the host with the system encoder → ffmpeg with a hardware encoder → ffmpeg in software |
| the words | the language heard first, then the model for it (`language.go`) → the general model |

## Rules that make it last

- The vault format is the product; the code around it is replaceable.
- Every recording Poiesis makes is H.264 video and AAC audio in mp4: it plays everywhere, and the video is
  the truth every page is rebuilt from.
- The core runs without a host, in any terminal, on every system; a host only makes it better.
- A host holds no log logic.
- A camera has one owner: the host that shows it also records it.
- A look is a colour matrix, defined once in the core and applied by every host.
- The app follows the desktop it lives in: on Omarchy the theme is to come from the desktop's
  own theme (planned, not built); elsewhere from the app's themes.
- Every seam has a test that runs without the real thing: a fake host for the contract, a
  synthetic camera for the encoder, a clip with a hole in its audio for the timeline.
- Costs are measured, not assumed; the numbers live next to the decisions.
- Nothing leaves the machine unless the person chose it.

## Where things live

    app/            the core (Go)
    app/docs/       the vault format, this page, the host contract
    app/hosts/contract/ the contract test every host runs: a fake core and the checks
    app/hosts/mac/      the Mac host (Swift)
    app/hosts/linux/    the Linux host (C, on GTK 4, VTE and GStreamer)
    app/hosts/windows/  the Windows host (planned)

## Per system

The choices behind the contract, each replaceable without touching the core:

| system | host | picture | recording | without a host |
|---|---|---|---|---|
| macOS | Swift and AppKit; the SwiftTerm text view with a transparent ground | the camera's frames, without a copy, in a display layer that is fed only when the picture moves | one capture session, its picture scaled to 1280x720, into an mp4 writer: hardware H.264, AAC | any terminal; Ghostty, kitty and WezTerm draw the picture |
| Linux, Omarchy first | GTK4 with the VTE text view, transparent ground, over a picture fed by GStreamer | PipeWire camera into GTK's video sink | the same pipeline, split: VA-API or NVENC H.264, x264 as fallback, AAC | the person's terminal: kitty graphics in Ghostty and kitty, sixel planned for Foot (Omarchy's default) |
| Windows | a Go program around the system web view: xterm.js with a transparent ground over the camera in a video element, the core in a pseudo-console | the web view's camera, on the graphics chip | the web view's recorder into mp4: hardware H.264, AAC | Windows Terminal; sixel planned, the picture in characters until then |

Why these: on each system the terminal view must let the camera show through its ground.
Research on 2026-09-23 found SwiftTerm and VTE do this; Windows' own terminal controls cannot
draw over video, so there the web view is the one that can. Memory for the text view and window
alone: about 75 MB for GTK4 and VTE, about 320 MB for a web view host.

## Adding a system

Write a host against `HOST.md`, make it pass the contract test in `hosts/contract`, bundle it
with the core. Nothing in the core changes.

## Building for another system on one machine

The core builds for every system from any one of them (`GOOS=linux` or `GOOS=windows`,
`CGO_ENABLED=0`; the Mac build needs cgo and a Mac). The Linux host is built and tested on any
machine with Docker, a Mac included: `hosts/linux/test.sh` runs Arch Linux, as Omarchy does,
with a headless Wayland desktop and a test picture and tone in place of the camera and the
microphone. What that cannot show is a real camera through PipeWire; that needs a Linux
machine.

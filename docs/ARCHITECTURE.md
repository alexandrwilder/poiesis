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
| the picture | drawn by the host → terminal graphics (the kitty protocol) → a picture in characters → off |
| the recording | made by the host with the system encoder → ffmpeg with a hardware encoder → ffmpeg in software |
| the words | the language heard first, then the model for it (`language.go`) → the general model |

## Rules that make it last

- The vault format is the product; the code around it is replaceable.
- Every recording is H.264 video and AAC audio in mp4: it plays everywhere, and the video is
  the truth every page is rebuilt from.
- The core runs without a host, in any terminal, on every system; a host only makes it better.
- A host holds no log logic.
- Every seam has a test that runs without the real thing: a fake host for the contract, a
  synthetic camera for the encoder, a clip with a hole in its audio for the timeline.
- Costs are measured, not assumed; the numbers live next to the decisions.
- Nothing leaves the machine unless the person chose it.

## Where things live

    app/            the core (Go)
    app/docs/       the vault format, this page, the host contract
    app/hosts/mac/  the Mac host (Swift)
    app/hosts/…     one folder per further system, each built against HOST.md only

## Per system

The choices behind the contract, each replaceable without touching the core:

| system | host | picture | recording | without a host |
|---|---|---|---|---|
| macOS | Swift and AppKit, a terminal view over the system camera layer | the system's preview layer, on the graphics chip | the system's hardware H.264 encoder | any terminal; Ghostty, kitty and WezTerm draw the picture |
| Linux, Omarchy first | to be decided from research | | | the person's own terminal |
| Windows | to be decided from research | | | Windows Terminal; the picture in characters |

## Adding a system

Write a host against `HOST.md`, make it pass the contract test, bundle it with the core.
Nothing in the core changes.

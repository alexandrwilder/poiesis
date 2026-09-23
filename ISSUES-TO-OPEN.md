# Issues to open on the first day of the repository

Small, described, and each one a real improvement. The ones marked *first* are for someone
who has never seen the code.

1. *first* **A theme of your own.** `theme.go` holds six. Add one with a name and eight
   colours; screenshot it in settings. Guidance: one accent that reads on black, a second
   colour for what is yours.
2. *first* **A look of your own.** `look.go`: a pixel in, a pixel out. Ideas: "print"
   (grain), "dusk" (orange to blue), "cold" (green-blue). Must pass `TestLooks`.
3. *first* **Weekday and month names in the log's language.** Day headings are English;
   the vault has a language setting. `dayHeading` in `tui_log.go`.
4. **Windows: the one-line installer.** `install.ps1` is a draft nobody has run. Run it on
   a Windows machine, make it true, report what the tray does there.
5. **Linux: the window on a real machine.** `hosts/linux` passes its tests in Arch under
   Docker, with a test picture and tone. Run it on Omarchy with a real camera through
   PipeWire, give it Omarchy's own theme, and package it for the AUR.
6. **A third extractor.** `extract.go`, the `Extractor` interface. llama.cpp directly, or
   any local server that speaks the OpenAI shape. Must score on the gold set (the maintainer runs it) before merge.
7. **Windows: the window.** `docs/ARCHITECTURE.md` has the plan: a Go program around the
   system web view, xterm.js over the camera in a video element. The core already builds for
   Windows; nothing has run there yet.
8. **The Map.** A screen of lanes over time, one per mission and one per person, with the
   claims as dots. The design is written; ask for it in the issue. The first version can be a static HTML page
   the app writes and opens.
9. **Mission lines that turn green.** A mission page may carry three lines of what to talk
   about; after processing, the ones talked about turn green on the entry page. Design in
   the maintainer's notes; ask for it in the issue.
10. **Swedish-only transcription from settings.** The KB-Whisper model exists behind
    `setup --swedish`; it should be a row on the settings page with its size and a
    download on first use.
11. **The entry page on the phone.** Not an app: a static HTML export of one entry with
    the video and the claims, to send to yourself. `entry_ops.go` is the place.
12. **Fewer files, same behaviour.** `write.go` does five things. Split it along the map in
    `CONTRIBUTING.md` without changing a byte of output; the tests must stay green.
13. **Package the tests' fixtures.** The tests that need a vault build one by hand each
    time; a `testvault()` helper with three entries would make new tests a few lines.
14. **Signing.** Sign and notarise the Mac release with a Developer ID in `release.sh`, so
    macOS opens it without the Terminal line and keeps the camera and microphone
    permissions across updates; document the steps so a fork can ship a signed build too.

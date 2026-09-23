# Poiesis

Talk to the camera for three minutes. Keep your own record. Let any AI read it.

Poiesis is a video log that lives on your computer. Each entry is a short recording of you
talking; the app turns it into words, then into claims, then into a log you can search,
and an AI you choose can read years of it in one go. Your entries stay on your computer:
the speech model and the local AI run inside the app. Your log is a folder of plain files.

**A preview.** The first release is for Macs with Apple silicon and macOS 14 or later. The
Linux window passes its tests but has not yet met a real camera; Windows is planned.

> screenshot of the record screen goes here: the picture edge to edge, the frame over it,
> the streak top right

## Install

Mac with Apple silicon and macOS 14 or later: paste one line into Terminal. It installs the
app, the `poiesis` command and the menu bar item, and checks every file first:

    curl -fsSL https://raw.githubusercontent.com/alexandrwilder/poiesis/main/install.sh | sh

Or download `Poiesis.dmg` from the newest release and drag Poiesis to Applications. Poiesis is
not signed by Apple yet, so a download from a browser needs the one line in its READ ME FIRST.

Linux: the same line installs the command. The Linux window is in `hosts/linux` (`make`).

Then open Poiesis. Press space, talk, press enter.

## Updates

While it is open, Poiesis looks every hour whether a newer version exists: it asks the
release page for the newest version number and sends nothing about you. A newer one shows in
the frame's corner; enter on the updates row in settings installs it, the same way Poiesis
was installed, and starts it again. Settings can turn the look off. `poiesis update` does it
from a terminal. Every download is checked against the release's checksums first.

## What you get

- **The record screen.** Camera on, a timer, your mission's name, the audio meter. Space
  starts, space pauses when life interrupts, enter completes. Three minutes at most.
- **The log.** Every entry under its day, a search bar that searches what you said, your
  missions as buttons. Enter opens an entry: the words with their times, the claims in
  the margin, and what you said earlier about the same people and things.
- **Your files.** `~/Documents/Poiesis Vault`: one markdown page per entry, one page per
  person, project or mission, and one file of claims. Obsidian opens it. So does grep.
- **The AI connection.** `poiesis setup --mcp` lets Claude Code read the log; other apps get
  their lines from `poiesis mcp --connect`. Four verbs, all read-only: orient, search, read,
  moment. Ask "what did I say about the bakery in August, and play the moment."

## The promise

Recording, speech to text and the extraction of claims run on your computer. Two things
reach the network: the models, downloaded once on first use, and the daily version check,
which settings can turn off. One choice sends words away: in settings you can switch
extraction to your own Claude key; then the words of each entry are sent to Anthropic under
your account. Video and sound never go anywhere. An AI app you connect reads the text it asks
for. Your folder follows your own backup and sync settings. The settings page says what is
on, in one line, at the top.

## The files, the format, the AI connection

`docs/FORMAT.md` explains the vault in plain words with examples. The format is MIT licensed
(`LICENSES/MIT.txt`), so any tool can read and write Poiesis vaults freely, your own AI
connection included. The app is AGPL-3.0 (`LICENSE`). The name and the mark are covered by
`TRADEMARK.md`. The tools inside the app and their licences are listed in `NOTICE.md`.

## Made of

Go, Bubble Tea for the screens, whisper.cpp for speech to text, Ollama with a small local
model for the claims, ffmpeg for video and sound. Each system gets a small window program
that draws the camera under the text and records it with the system's own encoder: Swift and
SwiftTerm on a Mac (`hosts/mac`), C on GTK 4, VTE and GStreamer on Linux (`hosts/linux`).
`docs/ARCHITECTURE.md` explains the layers and `docs/HOST.md` the contract between them. All
open source, all credited in `NOTICE.md`.

## Contributing

`CONTRIBUTING.md` has the map of the code, the four places made to be extended (themes,
looks, extractors, missions), and the one rule: nothing about extraction ships without a
score on the gold set.

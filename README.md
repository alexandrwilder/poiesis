# LOG_

Talk to the camera for three minutes. Keep your own record. Let any AI read it.

LOG_ is a video log that lives on your computer. Each entry is a short recording of you
talking; the app turns it into words, then into claims, then into a log you can search,
and an AI you choose can read years of it in one go. Nothing leaves your machine: the
speech model and the local AI run inside the app. Your log is a folder of plain files.

> screenshot of the record screen goes here: the picture edge to edge, the frame over it,
> the streak top right

## Install

Mac, one line:

    curl -fsSL https://raw.githubusercontent.com/alexandrwilder/log_/main/install.sh | sh

Mac with Homebrew: `brew install --cask alexandrwilder/tap/log_`. Omarchy and Arch: `yay -S log_-bin`.
Linux: the same one line as the Mac. Windows: `irm https://raw.githubusercontent.com/alexandrwilder/log_/main/install.ps1 | iex`.

Then open LOG_. Press space, talk, press enter.

## What you get

- **The record screen.** Camera on, a timer, your mission's name, the audio meter. Space
  starts, space pauses when life interrupts, enter completes. Three minutes at most.
- **The log.** Every entry under its day, a search bar that searches what you said, your
  missions as buttons. Enter opens an entry: the words with their times, the claims in
  the margin, and what you said earlier about the same people and things.
- **Your files.** `~/Documents/LOG_ Vault`: one markdown page per entry, one page per
  person, project or mission, and one file of claims. Obsidian opens it. So does grep.
- **The AI connection.** `log_ setup --mcp` lets Claude Code read the log; other apps get
  their lines from `log_ mcp --connect`. Four verbs, all read-only: orient, search, read,
  moment. Ask "what did I say about the bakery in August, and play the moment."

## The promise

Recording, speech to text and the extraction of claims run on your computer. The one
exception is a choice: in settings you can switch extraction to your own Claude key; then
the words of each entry are sent to Anthropic under your account. Video and sound never
go anywhere. The settings page says which is on, in one line, at the top.

## The files, the format, the AI connection

`docs/FORMAT.md` explains the vault in plain words with examples. The format and the AI
connection are MIT licensed so that other tools can read and write LOG_ vaults freely.
The app itself is AGPL-3.0. The tools inside the app and their licences are listed in
`NOTICE.md`.

## Made of

Go, Bubble Tea for the screens, ffmpeg for the camera, whisper.cpp for speech to text,
Ollama with a small local model for the claims, and on a Mac a renamed copy of the Ghostty
terminal as the window, because it can draw real pixels under text. All open source, all
credited in `NOTICE.md`.

## Contributing

`CONTRIBUTING.md` has the map of the code, the four places made to be extended (themes,
looks, extractors, missions), and the one rule: nothing about extraction ships without a
score on the gold set.

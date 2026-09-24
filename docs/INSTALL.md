# Installing Poiesis

## What it needs

- A Mac with Apple silicon (M1 or later) and macOS 14 or later.
- About 4 GB for the models, fetched once: the speech model (0.6 GB) with your first entry,
  and the local AI's model (3.4 GB) the first time an entry is sorted.
- About 31 MB for each three-minute entry, the video and the sound the words are heard from, so
  about 11 GB a year if you record every day.
- Your yes, once, for the camera and the microphone.

## The one line

    curl -fsSL https://raw.githubusercontent.com/alexandrwilder/poiesis/main/install.sh | sh

It downloads the newest release and checks every file against the release's checksums. It
puts Poiesis in `~/Applications`, the `poiesis` command in `~/.local/bin` and the menu bar item
at login, then opens Poiesis. Your log is made in `~/Poiesis Vault`, in your home folder, when
Poiesis first opens. A log made before 0.1 stays where it is, in `~/Documents/Poiesis Vault`.
Settings can move it, into iCloud Drive for example; then your phone can reach it too, and Apple
can read it unless Advanced Data Protection is on.

## The install file

Download `Poiesis.dmg` from the [newest release](https://github.com/alexandrwilder/poiesis/releases/latest)
and drag Poiesis to Applications. Poiesis is not signed by Apple yet, so macOS stops an app
that a browser downloaded, and the programs inside it. Paste the line from the install file's
READ ME FIRST into Terminal once:

    xattr -dr com.apple.quarantine /Applications/Poiesis.app

The one line above never needs this.

## Linux

The one line installs the `poiesis` command, and the log, search and the AI connection work.
Recording on Linux is not ready yet; its window lives in `hosts/linux`.

## Updates

While it is open, Poiesis looks every hour whether a newer version exists. A newer one shows
in the frame's corner, and enter on the updates row in settings installs it the same way
Poiesis was installed, then starts it again. `poiesis update` does the same from a terminal.
Every download is checked against the release's checksums first. Poiesis is signed with its
own certificate, so macOS keeps the camera and microphone approved across updates.

## What reaches the network

- **The models, once:** from Hugging Face and from Ollama's library, each checked before use.
- **The version check, every hour while Poiesis is open:** one request to github.com for the
  newest version number. Nothing about you is sent. Settings can turn it off.
- **Only if you add your own Claude key:** each entry's words go to Anthropic under your
  account. Never the video.
- **Only if you connect an AI app:** it reads the text it asks for. An AI that runs in the
  cloud, such as Claude or ChatGPT, sends that text to its company, as it does with anything
  you type to it. Never the video.
- **Your log folder stays on this Mac,** unless you move it into a synced folder such as
  iCloud Drive. Time Machine backs it up.

## Remove Poiesis

    launchctl bootout gui/$(id -u)/app.poiesis.tray 2>/dev/null
    rm -f ~/Library/LaunchAgents/app.poiesis.tray.plist
    rm -rf ~/Applications/Poiesis.app /Applications/Poiesis.app ~/.local/bin/poiesis
    rm -rf ~/Library/Application\ Support/Poiesis    # the models and the app's own settings

Your log stays in `~/Poiesis Vault` (or `~/Documents/Poiesis Vault`, from before 0.1): plain files, yours. Delete that folder only if
you want the entries gone too.

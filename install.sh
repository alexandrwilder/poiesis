#!/bin/sh
# Poiesis · one line installs it:
#
#     curl -fsSL https://raw.githubusercontent.com/alexandrwilder/poiesis/main/install.sh | sh
#
# Mac: the whole app (recording, speech to text and the local AI inside), the `poiesis`
# command, and the menu bar item at login. Linux: the command, then `poiesis setup` tells you
# the one package line your system needs. Everything lands in your home folder; nothing
# is sent anywhere. Set POIESIS_LOCAL_DMG or POIESIS_LOCAL_BINARY to install a build of your own,
# and POIESIS_NO_START=1 to install without starting anything.
#
# The files come from the project's newest release, the same ones the app updates itself from:
# Poiesis.dmg, poiesis_linux_<arch>.tar.gz and checksums.txt. A download that does not match its
# checksum is not installed; a new app replaces the old one only once it is whole.
set -eu

REPO="${POIESIS_REPO:-alexandrwilder/poiesis}"
RELEASES="${POIESIS_RELEASES:-https://github.com/$REPO/releases}"
BIN_DIR="${POIESIS_BIN_DIR:-$HOME/.local/bin}"
say() { printf '%s\n' "$*"; }

os=$(uname -s)
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) say "Poiesis has no build for this processor ($arch) yet"; exit 1 ;;
esac
mkdir -p "$BIN_DIR"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

case "$os" in
Darwin)
  [ "$arch" = arm64 ] || { say "Poiesis for Mac needs Apple silicon for now"; exit 1; }
  dmg="${POIESIS_LOCAL_DMG:-}"
  if [ -z "$dmg" ]; then
    say "downloading Poiesis from $RELEASES/latest"
    curl -fL# "$RELEASES/latest/download/Poiesis.dmg" -o "$tmp/Poiesis.dmg"
    curl -fsSL "$RELEASES/latest/download/checksums.txt" -o "$tmp/checksums.txt"
    (cd "$tmp" && grep " Poiesis.dmg\$" checksums.txt | shasum -a 256 -c - >/dev/null) || { say "the download does not match its checksum; not installing"; exit 1; }
    dmg="$tmp/Poiesis.dmg"
  fi
  dest="$HOME/Applications"
  mkdir -p "$dest"
  mnt=$(hdiutil attach -nobrowse -readonly "$dmg" | grep -o '/Volumes/.*' | head -1)
  [ -n "$mnt" ] || { say "could not open the install file"; exit 1; }
  # the new app goes beside the old one, and replaces it only once it is whole
  new="$dest/.Poiesis-new.app"
  rm -rf "$new"
  ditto "$mnt/Poiesis.app" "$new"
  hdiutil detach "$mnt" -quiet
  # you chose to install it: without this macOS stops the app and the programs inside it
  xattr -dr com.apple.quarantine "$new" 2>/dev/null || true
  codesign --verify --deep --strict "$new" 2>/dev/null || { rm -rf "$new"; say "the app in the install file is not whole; not installing"; exit 1; }
  # a vault chosen inside the old app moves to the app's own folder, which updates keep
  state="$HOME/Library/Application Support/Poiesis"
  if [ -f "$dest/Poiesis.app/Contents/Resources/vault.txt" ] && [ ! -f "$state/vault.txt" ]; then
    mkdir -p "$state" && cp "$dest/Poiesis.app/Contents/Resources/vault.txt" "$state/vault.txt"
  fi
  rm -rf "$dest/.Poiesis-previous.app"
  if [ -d "$dest/Poiesis.app" ]; then mv "$dest/Poiesis.app" "$dest/.Poiesis-previous.app"; fi
  if ! mv "$new" "$dest/Poiesis.app"; then
    if [ -d "$dest/.Poiesis-previous.app" ]; then mv "$dest/.Poiesis-previous.app" "$dest/Poiesis.app"; fi
    say "could not put the new app in place; the old one is back"; exit 1
  fi
  rm -rf "$dest/.Poiesis-previous.app"
  ln -sf "$dest/Poiesis.app/Contents/MacOS/poiesis" "$BIN_DIR/poiesis"
  if [ -n "${POIESIS_NO_START:-}" ]; then say "installed $dest/Poiesis.app"; exit 0; fi
  "$dest/Poiesis.app/Contents/Library/LoginItems/Poiesis Menu.app/Contents/MacOS/poiesis" tray --login >/dev/null 2>&1 || true
  say ""
  say "Poiesis is installed."
  say "  the app:       $dest/Poiesis.app   (Launchpad, Spotlight, the Dock)"
  say "  the command:   poiesis"
  say "  the menu bar:  starts at every login"
  say "  your log:      ~/Documents/Poiesis Vault   (plain files, yours)"
  say ""
  say "opening it now. macOS will ask once for the Documents folder, the camera and the microphone."
  open "$dest/Poiesis.app"
  ;;
Linux)
  if [ -n "${POIESIS_LOCAL_BINARY:-}" ]; then
    cp "$POIESIS_LOCAL_BINARY" "$BIN_DIR/poiesis"
  else
    say "downloading Poiesis from $RELEASES/latest"
    curl -fsSL "$RELEASES/latest/download/poiesis_linux_${arch}.tar.gz" -o "$tmp/poiesis.tar.gz"
    curl -fsSL "$RELEASES/latest/download/checksums.txt" -o "$tmp/checksums.txt"
    (cd "$tmp" && grep " poiesis_linux_${arch}.tar.gz\$" checksums.txt | sha256sum -c - >/dev/null) || { say "the download does not match its checksum; not installing"; exit 1; }
    tar -xzf "$tmp/poiesis.tar.gz" -C "$tmp" poiesis
    mv "$tmp/poiesis" "$BIN_DIR/poiesis"
  fi
  chmod +x "$BIN_DIR/poiesis"
  say "installed $BIN_DIR/poiesis"
  if [ -z "${POIESIS_NO_START:-}" ]; then "$BIN_DIR/poiesis" setup; fi
  ;;
*)
  say "on Windows, open PowerShell and run:  irm https://raw.githubusercontent.com/$REPO/main/install.ps1 | iex"
  exit 1
  ;;
esac

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) say "to use the command from any terminal, add this line to your shell profile:  export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac

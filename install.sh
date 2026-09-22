#!/bin/sh
# Poiesis · one line installs it:
#
#     curl -fsSL https://raw.githubusercontent.com/alexandrwilder/poiesis/main/install.sh | sh
#
# Mac: the whole app (recording, speech to text and the local AI inside), the `poiesis`
# command, and the menu bar item at login. Linux: the command, then `poiesis setup` tells you
# the one package line your system needs. Everything lands in your home folder; nothing
# is sent anywhere. Set POIESIS_LOCAL_DMG or POIESIS_LOCAL_BINARY to install a build of your own.
set -eu

REPO="${POIESIS_REPO:-alexandrwilder/poiesis}"
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
    url="https://github.com/$REPO/releases/latest/download/Poiesis.dmg"
    say "downloading Poiesis from $url"
    curl -fL# "$url" -o "$tmp/Poiesis.dmg"
    dmg="$tmp/Poiesis.dmg"
  fi
  dest="$HOME/Applications"
  mkdir -p "$dest"
  mnt=$(hdiutil attach -nobrowse -readonly "$dmg" | grep -o '/Volumes/.*' | head -1)
  [ -n "$mnt" ] || { say "could not open the install file"; exit 1; }
  rm -rf "$dest/Poiesis.app"
  ditto "$mnt/Poiesis.app" "$dest/Poiesis.app"
  hdiutil detach "$mnt" -quiet
  # you chose to install it: without this macOS would ask for right-click → Open once
  xattr -dr com.apple.quarantine "$dest/Poiesis.app" 2>/dev/null || true
  ln -sf "$dest/Poiesis.app/Contents/MacOS/poiesis" "$BIN_DIR/poiesis"
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
    url="https://github.com/$REPO/releases/latest/download/poiesis_linux_${arch}.tar.gz"
    say "downloading Poiesis from $url"
    curl -fsSL "$url" -o "$tmp/poiesis.tar.gz"
    curl -fsSL "https://github.com/$REPO/releases/latest/download/checksums.txt" -o "$tmp/checksums.txt"
    (cd "$tmp" && grep "poiesis_linux_${arch}.tar.gz" checksums.txt | sha256sum -c - >/dev/null) || { say "checksum mismatch; not installing"; exit 1; }
    tar -xzf "$tmp/poiesis.tar.gz" -C "$tmp" poiesis
    mv "$tmp/poiesis" "$BIN_DIR/poiesis"
  fi
  chmod +x "$BIN_DIR/poiesis"
  say "installed $BIN_DIR/poiesis"
  "$BIN_DIR/poiesis" setup
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

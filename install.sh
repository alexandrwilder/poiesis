#!/bin/sh
# LOG_ · one line installs it:
#
#     curl -fsSL https://raw.githubusercontent.com/alexandrwilder/log_/main/install.sh | sh
#
# Mac: the whole app (recording, speech to text and the local AI inside), the `log_`
# command, and the menu bar item at login. Linux: the command, then `log_ setup` tells you
# the one package line your system needs. Everything lands in your home folder; nothing
# is sent anywhere. Set LOG_LOCAL_DMG or LOG_LOCAL_BINARY to install a build of your own.
set -eu

REPO="${LOG_REPO:-alexandrwilder/log_}"
BIN_DIR="${LOG_BIN_DIR:-$HOME/.local/bin}"
say() { printf '%s\n' "$*"; }

os=$(uname -s)
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) say "LOG_ has no build for this processor ($arch) yet"; exit 1 ;;
esac
mkdir -p "$BIN_DIR"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

case "$os" in
Darwin)
  [ "$arch" = arm64 ] || { say "LOG_ for Mac needs Apple silicon for now"; exit 1; }
  dmg="${LOG_LOCAL_DMG:-}"
  if [ -z "$dmg" ]; then
    url="https://github.com/$REPO/releases/latest/download/LOG_.dmg"
    say "downloading LOG_ from $url"
    curl -fL# "$url" -o "$tmp/LOG_.dmg"
    dmg="$tmp/LOG_.dmg"
  fi
  dest="$HOME/Applications"
  mkdir -p "$dest"
  mnt=$(hdiutil attach -nobrowse -readonly "$dmg" | grep -o '/Volumes/.*' | head -1)
  [ -n "$mnt" ] || { say "could not open the install file"; exit 1; }
  rm -rf "$dest/LOG_.app"
  ditto "$mnt/LOG_.app" "$dest/LOG_.app"
  hdiutil detach "$mnt" -quiet
  # you chose to install it: without this macOS would ask for right-click → Open once
  xattr -dr com.apple.quarantine "$dest/LOG_.app" 2>/dev/null || true
  ln -sf "$dest/LOG_.app/Contents/MacOS/log_" "$BIN_DIR/log_"
  "$dest/LOG_.app/Contents/Library/LoginItems/LOG_ Menu.app/Contents/MacOS/log_" tray --login >/dev/null 2>&1 || true
  say ""
  say "LOG_ is installed."
  say "  the app:       $dest/LOG_.app   (Launchpad, Spotlight, the Dock)"
  say "  the command:   log_"
  say "  the menu bar:  starts at every login"
  say "  your log:      ~/Documents/LOG_ Vault   (plain files, yours)"
  say ""
  say "opening it now. macOS will ask once for the Documents folder, the camera and the microphone."
  open "$dest/LOG_.app"
  ;;
Linux)
  if [ -n "${LOG_LOCAL_BINARY:-}" ]; then
    cp "$LOG_LOCAL_BINARY" "$BIN_DIR/log_"
  else
    url="https://github.com/$REPO/releases/latest/download/log__linux_${arch}.tar.gz"
    say "downloading LOG_ from $url"
    curl -fsSL "$url" -o "$tmp/log_.tar.gz"
    curl -fsSL "https://github.com/$REPO/releases/latest/download/checksums.txt" -o "$tmp/checksums.txt"
    (cd "$tmp" && grep "log__linux_${arch}.tar.gz" checksums.txt | sha256sum -c - >/dev/null) || { say "checksum mismatch; not installing"; exit 1; }
    tar -xzf "$tmp/log_.tar.gz" -C "$tmp" log_
    mv "$tmp/log_" "$BIN_DIR/log_"
  fi
  chmod +x "$BIN_DIR/log_"
  say "installed $BIN_DIR/log_"
  "$BIN_DIR/log_" setup
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

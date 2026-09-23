#!/bin/sh
# Makes a release from this Mac. It publishes the files that install.sh installs and that the
# app updates itself from:
#   Poiesis.dmg                        the Mac app (Apple silicon, macOS 14 or later)
#   poiesis_linux_amd64.tar.gz, _arm64 the command on its own for Linux
#   checksums.txt                      SHA-256 of the three, which every install checks
#   release.json                       the facts the download page shows
#
#     ./release.sh 0.0.6 notes.md       notes.md says what changed, in plain words
#
# It needs a clean tree that GitHub already has, Go, Xcode's command line tools (for the Mac
# host) and gh signed in. The version is stamped into every build. POIESIS_RELEASE_DRY=1
# builds and checks everything but publishes nothing.
set -eu
cd "$(dirname "$0")"
usage="usage: ./release.sh <version, like 0.0.6> <notes file>"
ver="${1:?$usage}"
notes="${2:?$usage}"
case "$ver" in v*) echo "give the version without the v: ${ver#v}"; exit 1 ;; esac
[ -f "$notes" ] || { echo "no notes file at $notes"; exit 1; }
notes="$(cd "$(dirname "$notes")" && pwd)/$(basename "$notes")"
if [ -z "${POIESIS_RELEASE_DRY:-}" ]; then
  [ -z "$(git status --porcelain)" ] || { echo "commit first: a release is made from a clean tree"; exit 1; }
  git fetch -q origin
  [ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || { echo "push first: the release is tagged on what GitHub has"; exit 1; }
fi
out="$HOME/Library/Caches/Poiesis/release-$ver" # outside iCloud, which marks files
rm -rf "$out"
mkdir -p "$out"
stamp="-X main.version=$ver"

# Signed with Poiesis's own certificate, so macOS keeps the app's permissions across updates.
# Its keychain joins the search list only while this runs, then the list is put back.
KC="$HOME/Library/Keychains/poiesis-signing.keychain-db"
[ -f "$KC" ] || { echo "no signing identity: run hosts/mac/make-signing-identity.sh once"; exit 1; }
keychains=$(security list-keychains -d user | tr -d '"' | xargs)
trap 'security list-keychains -d user -s $keychains' EXIT
# shellcheck disable=SC2086
security list-keychains -d user -s $keychains "$KC"
security unlock-keychain -p "$(security find-generic-password -s poiesis-signing -w)" "$KC"
export POIESIS_SIGN_IDENTITY="Poiesis (self-signed)"

echo "== the Mac host"
hosts/mac/build.sh >/dev/null
echo "== the Mac app and its install file"
go build -ldflags "$stamp" -o "$out/poiesis" .
cp -R assets "$out/assets" # the app's icon is found beside the program
"$out/poiesis" dmg "$out/Poiesis.dmg" >/dev/null
echo "== the command for Linux"
for arch in amd64 arm64; do
  mkdir -p "$out/linux-$arch"
  GOOS=linux GOARCH=$arch CGO_ENABLED=0 go build -ldflags "$stamp" -o "$out/linux-$arch/poiesis" .
  tar -C "$out/linux-$arch" -czf "$out/poiesis_linux_$arch.tar.gz" poiesis
done

echo "== checksums and the page's facts"
repo="${POIESIS_REPO:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}"
(
  cd "$out"
  shasum -a 256 Poiesis.dmg poiesis_linux_amd64.tar.gz poiesis_linux_arm64.tar.gz > checksums.txt
  shasum -a 256 -c checksums.txt >/dev/null
  cat > release.json <<JSON
{
  "status": "released",
  "version": "$ver",
  "platform": "Mac",
  "requirements": "Apple silicon (M1 or later), macOS 14 or later",
  "url": "https://github.com/$repo/releases/download/v$ver/Poiesis.dmg",
  "sha256": "$(grep ' Poiesis.dmg$' checksums.txt | cut -d' ' -f1)",
  "size": $(stat -f %z Poiesis.dmg),
  "repo": "$repo",
  "install": "curl -fsSL https://raw.githubusercontent.com/$repo/main/install.sh | sh"
}
JSON
  python3 -m json.tool release.json >/dev/null
)
[ "$("$out/poiesis" version)" = "poiesis $ver" ] || { echo "the Mac build says $("$out/poiesis" version), not $ver"; exit 1; }
# the app in the install file must carry the certificate, or updates would ask for permissions again
mnt=$(hdiutil attach -nobrowse -readonly "$out/Poiesis.dmg" | grep -o '/Volumes/.*' | head -1)
req=$(codesign -d -r- "$mnt/Poiesis.app" 2>&1 | grep designated || true)
hdiutil detach "$mnt" -quiet
case "$req" in
  *"certificate leaf"*) echo "   signed as Poiesis: $req" ;;
  *) echo "the app is not signed with Poiesis's certificate: $req"; exit 1 ;;
esac
ls -l "$out/Poiesis.dmg" "$out"/poiesis_linux_*.tar.gz | awk '{print "   " $5 "  " $NF}'

if [ -n "${POIESIS_RELEASE_DRY:-}" ]; then
  echo "dry run: nothing published; the files are in $out"
  exit 0
fi
echo "== the release"
gh release create "v$ver" --repo "$repo" --target main --title "Poiesis $ver" --notes-file "$notes" \
  "$out/Poiesis.dmg" "$out/poiesis_linux_amd64.tar.gz" "$out/poiesis_linux_arm64.tar.gz" \
  "$out/checksums.txt" "$out/release.json"

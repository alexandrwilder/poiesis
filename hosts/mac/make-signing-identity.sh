#!/bin/sh
# Makes Poiesis's own code-signing identity, once, on the Mac that makes the releases.
#
# A self-signed certificate costs nothing and gives the app a stable identity: macOS knows an
# update as the same app and keeps camera, microphone and Documents approved. It is not
# Apple's Developer ID, so a download from a browser is still stopped until the READ ME's line.
#
# The key lives in a keychain of its own; that keychain's password lives in your login
# keychain. Back up the .p12 this prints, and its password
# (security find-generic-password -s poiesis-signing -w), in your password manager: a lost key
# means one more round of permission questions for everyone.
set -eu
NAME="Poiesis (self-signed)"
KC="$HOME/Library/Keychains/poiesis-signing.keychain-db"
DIR="$HOME/Library/Application Support/Poiesis/signing" # not in iCloud
if [ -f "$KC" ]; then
  echo "already made: $KC"
  exit 0
fi
OPENSSL=/opt/homebrew/bin/openssl
[ -x "$OPENSSL" ] || OPENSSL=openssl
mkdir -p "$DIR"
chmod 700 "$DIR"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
PW=$("$OPENSSL" rand -hex 24)

# twenty years, for signing code only
"$OPENSSL" req -x509 -newkey rsa:3072 -sha256 -days 7300 -nodes -subj "/CN=$NAME" \
  -addext "basicConstraints=critical,CA:false" -addext "keyUsage=critical,digitalSignature" \
  -addext "extendedKeyUsage=critical,codeSigning" \
  -keyout "$tmp/key.pem" -out "$tmp/cert.pem" 2>/dev/null
# macOS reads only the older .p12 encryption
"$OPENSSL" pkcs12 -export -legacy -name "$NAME" -inkey "$tmp/key.pem" -in "$tmp/cert.pem" \
  -out "$DIR/poiesis-signing.p12" -passout "pass:$PW"
chmod 600 "$DIR/poiesis-signing.p12"

security create-keychain -p "$PW" "$KC"
security set-keychain-settings -l -u -t 3600 "$KC" # locks after an hour and on sleep
security unlock-keychain -p "$PW" "$KC"
security import "$DIR/poiesis-signing.p12" -k "$KC" -P "$PW" -T /usr/bin/codesign >/dev/null
security set-key-partition-list -S apple-tool:,apple: -s -k "$PW" "$KC" >/dev/null
security add-generic-password -U -a "$USER" -s poiesis-signing -w "$PW" -T /usr/bin/security \
  "$HOME/Library/Keychains/login.keychain-db"
# codesign uses only identities macOS trusts: trust this one for code signing, nothing else.
# macOS asks for your password once, in a dialog.
security find-certificate -c "$NAME" -p "$KC" > "$DIR/poiesis-signing.cer.pem"
security add-trusted-cert -r trustRoot -p codeSign -k "$KC" "$DIR/poiesis-signing.cer.pem"
echo "made \"$NAME\" in $KC"
echo "back up $DIR/poiesis-signing.p12 and its password in your password manager"

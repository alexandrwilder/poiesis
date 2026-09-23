#!/bin/sh
# Builds the Mac host and a test app with the core inside. The build lives outside Documents,
# which iCloud syncs and marks. The real app bundle is made by `poiesis setup --app`.
set -eu
cd "$(dirname "$0")"
OUT="${POIESIS_HOST_BUILD:-$HOME/Library/Caches/Poiesis/host-build}"
swift build -c release --scratch-path "$OUT" 2>&1 | tail -3
BIN="$OUT/release/PoiesisHost"
APP="$OUT/Poiesis Host.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BIN" "$APP/Contents/MacOS/PoiesisHost"
if [ -d "$OUT/release/SwiftTerm_SwiftTerm.bundle" ]; then cp -R "$OUT/release/SwiftTerm_SwiftTerm.bundle" "$APP/Contents/Resources/"; fi
# the core is built here too, so the test app never carries an old one
(cd ../.. && go build -o poiesis .) || exit 1
CORE="${POIESIS_CORE:-../../poiesis}"
if [ -x "$CORE" ]; then cp "$CORE" "$APP/Contents/MacOS/poiesis"; fi
cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key><string>PoiesisHost</string>
  <key>CFBundleIdentifier</key><string>app.poiesis.host-test</string>
  <key>CFBundleName</key><string>Poiesis</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>0.1</string>
  <key>CFBundleVersion</key><string>1</string>
  <key>LSMinimumSystemVersion</key><string>14.0</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSCameraUsageDescription</key><string>Poiesis shows you while you talk and records the entry when you press space. Nothing leaves this computer.</string>
  <key>NSMicrophoneUsageDescription</key><string>Poiesis records your voice with each entry. Nothing leaves this computer.</string>
</dict>
</plist>
PLIST
cat > "$OUT/host.entitlements" <<ENT
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>com.apple.security.device.camera</key><true/>
  <key>com.apple.security.device.audio-input</key><true/>
</dict>
</plist>
ENT
chmod -R u+w "$APP"
xattr -cr "$APP" # files copied out of Documents carry attributes that signing refuses
if [ -x "$APP/Contents/MacOS/poiesis" ]; then codesign --force --sign - "$APP/Contents/MacOS/poiesis"; fi
codesign --force --sign - --options runtime --entitlements "$OUT/host.entitlements" "$APP"
echo "$APP"

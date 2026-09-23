#!/bin/sh
# The host's side of the contract, with the camera and microphone: runs the host with the
# fake core in ../contract/fake_core.py, then ../contract/check.py checks what came back and
# the recording itself. Run it from a terminal that may use the camera; the window shows for
# about ten seconds.
set -u
cd "$(dirname "$0")"
OUT="$(mktemp -d /tmp/phost.XXXX)"
APP=$(./build.sh | tail -1) || { echo "FAIL: build"; exit 1; }
"$APP/Contents/MacOS/PoiesisHost" --core "$(command -v python3)" -- "$PWD/../contract/fake_core.py" "$OUT/report.json" "$OUT/part01.mp4" &
HP=$!
n=0; while kill -0 $HP 2>/dev/null && [ $n -lt 60 ]; do sleep 1; n=$((n+1)); done
kill $HP 2>/dev/null
python3 ../contract/check.py "$OUT" Mac

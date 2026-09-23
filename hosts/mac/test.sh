#!/bin/sh
# The host's side of the contract, with the camera and microphone: runs the host with the
# fake core in contract_test.py, then checks what came back and the recording itself.
# Run it from a terminal that may use the camera; the window shows for about ten seconds.
set -u
cd "$(dirname "$0")"
OUT="$(mktemp -d /tmp/phost.XXXX)"
APP=$(./build.sh | tail -1) || { echo "FAIL: build"; exit 1; }
"$APP/Contents/MacOS/PoiesisHost" --core "$(command -v python3)" -- "$PWD/contract_test.py" "$OUT/report.json" "$OUT/part01.mp4" &
HP=$!
n=0; while kill -0 $HP 2>/dev/null && [ $n -lt 60 ]; do sleep 1; n=$((n+1)); done
kill $HP 2>/dev/null
python3 - "$OUT" <<'PY'
import json, os, subprocess, sys
out = sys.argv[1]
fails = []
try:
    r = json.load(open(os.path.join(out, "report.json")))
except Exception as e:
    print("FAIL: no report:", e); sys.exit(1)
if r.get("crash"):
    fails.append("the fake core crashed: " + r["crash"])
h = r.get("hello") or {}
if h.get("version") != 1 or not {"picture", "look", "record", "level"} <= set(h.get("can", [])):
    fails.append(f"hello: {h}")
if (r.get("started") or {}).get("state") != "started":
    fails.append(f"no started: {r.get('started')}")
stopped = r.get("stopped") or {}
if stopped.get("state") != "stopped" or not 4 <= stopped.get("seconds", 0) <= 7:
    fails.append(f"stopped: {stopped}")
if r.get("levels", 0) < 20:
    fails.append(f"only {r.get('levels')} meter levels in about eight seconds")
if r.get("errors"):
    fails.append(f"errors: {r['errors']}")
mp4 = os.path.join(out, "part01.mp4")
probe = subprocess.run(["ffprobe", "-v", "error", "-count_packets", "-show_entries",
                        "stream=codec_name,codec_type,width,height,sample_rate,channels,nb_read_packets,duration,avg_frame_rate",
                        "-of", "json", mp4], capture_output=True, text=True)
try:
    streams = {s["codec_type"]: s for s in json.loads(probe.stdout)["streams"]}
except Exception:
    streams = {}
    fails.append(f"no recording: {probe.stderr.strip()}")
v, a = streams.get("video", {}), streams.get("audio", {})
if streams:
    if v.get("codec_name") != "h264" or a.get("codec_name") != "aac" or int(a.get("channels", 0)) != 1:
        fails.append(f"streams: video {v.get('codec_name')}, audio {a.get('codec_name')} x{a.get('channels')}")
    ad, vd = float(a.get("duration", 0)), float(v.get("duration", 0))
    need = ad * int(a.get("sample_rate", 48000)) / 1024
    got = int(a.get("nb_read_packets", 0))
    print(f"recording: {v.get('width')}x{v.get('height')} h264 {v.get('avg_frame_rate')}, {vd:.2f} s video, {ad:.2f} s audio, {got} audio packets of {need:.0f}")
    if need and got < 0.98 * need:
        fails.append(f"audio lost: {got} of {need:.0f} packets")
    if abs(ad - vd) > 0.3:
        fails.append(f"audio {ad:.2f} s against video {vd:.2f} s")
print(f"meter levels: {r.get('levels')}, started after {r.get('started_after_s')} s, stop took {r.get('stop_took_s')} s")
if fails:
    print("FAIL:"); [print("  -", f) for f in fails]; sys.exit(1)
print("PASS: the Mac host keeps the contract and records a clean file")
PY

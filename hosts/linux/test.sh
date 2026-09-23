#!/bin/sh
# The Linux host on any machine with Docker, a Mac included: Arch Linux as Omarchy runs it, a
# headless Wayland desktop (sway), a moving test picture and a tone in place of the camera and
# the microphone. Builds the host, checks the look's colour matrix in GTK, runs the host with
# the fake core in ../contract, takes a screenshot while it records, then ../contract/check.py
# checks what came back and the recording itself. Nothing here needs a camera.
set -u
cd "$(dirname "$0")"
OUT="$(mktemp -d /tmp/plinux.XXXX)" # outside iCloud: Docker cannot read synced folders
docker build -q --platform linux/amd64 -t poiesis-linux-host -f Containerfile . >/dev/null || { echo "FAIL: the Linux image"; exit 1; }
# the sources go in as a stream, for the same reason; sway asks for SYS_NICE when it starts
COPYFILE_DISABLE=1 tar --no-xattrs --no-fflags --no-mac-metadata -C .. -cf - linux contract |
docker run --rm -i --platform linux/amd64 --cap-add SYS_NICE -v "$OUT":/out poiesis-linux-host sh -c '
  set -u
  mkdir -p /tmp/b && tar -C /tmp/b -xf - && cd /tmp/b/linux || exit 1
  make -s poiesis-host look-test || { echo "FAIL: build"; exit 1; }
  export XDG_RUNTIME_DIR=/tmp/xdg WLR_BACKENDS=headless WLR_LIBINPUT_NO_DEVICES=1 WLR_RENDERER=pixman
  mkdir -p -m 0700 "$XDG_RUNTIME_DIR"
  printf "output HEADLESS-1 resolution 1100x720\ndefault_border none\n" > /tmp/sway.conf
  sway -c /tmp/sway.conf > /out/sway.log 2>&1 &
  n=0; while [ ! -S "$XDG_RUNTIME_DIR/wayland-1" ] && [ $n -lt 50 ]; do sleep 0.2; n=$((n+1)); done
  export WAYLAND_DISPLAY=wayland-1 XDG_CURRENT_DESKTOP=sway
  ./look-test > /out/look.txt 2>&1
  POIESIS_HOST_SOURCES=test ./poiesis-host --core "$(command -v python3)" -- /tmp/b/contract/fake_core.py \
    /out/report.json /out/part01.mp4 > /out/host.log 2>&1 &
  HP=$!
  # the window while it records: the fake core marks when the recording has started
  n=0; while [ ! -e /out/report.json.recording ] && kill -0 $HP 2>/dev/null && [ $n -lt 300 ]; do sleep 0.1; n=$((n+1)); done
  sleep 1; grim /out/window.png 2>> /out/host.log
  n=0; while kill -0 $HP 2>/dev/null && [ $n -lt 60 ]; do sleep 1; n=$((n+1)); done
  kill $HP 2>/dev/null
  exit 0
'
echo "== the look in GTK"; grep -v "^MESA" "$OUT/look.txt" 2>/dev/null
echo "== the window while recording: $OUT/window.png"
# The test picture is black with a white ball. Through the fake core's warm look black becomes
# (3,0,0): that colour filling the window proves the camera is drawn behind the text, the text
# view's ground is see-through, and the look is applied. The theme's white means it is not.
ffmpeg -v error -i "$OUT/window.png" -vf format=rgb24 -f rawvideo - | python3 -c '
import collections, sys
d = sys.stdin.buffer.read()
common = collections.Counter(d[i:i + 3] for i in range(0, len(d) - 2, 3 * 7)).most_common(1)
r, g, b = common[0][0] if common else (255, 255, 255)
ok = 1 <= r <= 6 and g <= 2 and b <= 2
print(("the camera shows through, with the look" if ok else "FAIL: the camera does not show through") + f": most of the window is ({r},{g},{b})")
sys.exit(0 if ok else 1)' || SCREEN=FAIL
grep -q "^PASS" "$OUT/look.txt" 2>/dev/null || LOOK=FAIL
python3 ../contract/check.py "$OUT" Linux && [ "${LOOK:-}" != FAIL ] && [ "${SCREEN:-}" != FAIL ]

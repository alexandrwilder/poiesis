# The host contract

Version 1. A host is a small native program that owns a window, the camera and the
recording, and runs the core inside a text view. This page is everything the two agree on;
nothing outside it may be assumed by either side.

## Starting

The host makes a local socket in the app's own state folder, readable and writable only by
the user, and starts the core in its text view with the socket's path in the environment:

    POIESIS_HOST=/Users/you/Library/Application Support/Poiesis/host.sock   (macOS)
    POIESIS_HOST=/run/user/1000/poiesis/host.sock                          (Linux: $XDG_RUNTIME_DIR)

The core connects once, at start. Unix domain sockets work on macOS, Linux and on Windows 10
(1803) and later. Without `POIESIS_HOST`, or when the connection fails, the core runs on its
own and uses its ladders (`ARCHITECTURE.md`).

The host gives the core a whole environment, not only the socket: `PATH` (the system's and the
app's own tools), `HOME`, `LANG`, `TERM=xterm-256color`, `COLORTERM=truecolor`. The text view
supports 24-bit colour, the alternate screen, mouse reporting, focus reporting and bracketed
paste. Its background is transparent wherever the core draws no background colour; cells with
their own colour stay solid.

## Messages

One JSON object per line, UTF-8, both ways. Every message has a type in `t`. A side ignores
types and fields it does not know. Fields are only ever added.

## Meeting

The first message each way:

    core → host   {"t":"hello","versions":[1],"app":"poiesis 0.0.5"}
    host → core   {"t":"hello","version":1,"host":"poiesis-mac 0.1","can":["picture","look","record","level"]}

`can` lists what the host does. The core uses nothing that is not in it. With no shared
version the host answers `"version":0`, and the core goes on without the host.

## The picture (`picture`, `look`)

    core → host   {"t":"picture","on":true,"look":"warm","strength":0.4,
                   "matrix":[1.03,0,0,0.01, 0,0.98,0,0, 0,0,0.89,0],"fps":15}
    core → host   {"t":"picture","on":false}

On: the camera fills the window behind the text view, cropped to the window, never
stretched. Off: the picture rests; the camera stops unless something records. `fps` is a
wish; the host picks the nearest rate its camera offers.

`matrix` is the look: three rows for red, green and blue, each the weights of red, green
and blue and an offset, on a 0..1 scale, the result clamped to 0..1. Every graphics system
applies such a matrix (Core Image, GTK, a web view's colour matrix filter), so a host never
needs to know a look by name, and a new look in the core reaches every host at once. `look`
and `strength` are its name, for showing. A host that cannot apply a matrix shows the camera
as it is. The look changes only what is shown, never the recording.

## One owner for the camera

A host that says `picture` should also say `record`. On Linux and Windows a camera usually
serves one program at a time, so the host keeps the camera and records from the same capture;
the core never opens the camera while a host holds it.

## The recording (`record`)

    core → host   {"t":"record","action":"start","file":"/…/inbox/.parts/2026-09-23T21-06-58.part01.mp4"}
    host → core   {"t":"recording","state":"started"}
    core → host   {"t":"record","action":"stop"}
    host → core   {"t":"recording","state":"stopped","file":"/…part01.mp4","seconds":62.4}

One file per part: pausing is a stop, resuming a new start, and the core joins the parts as
it always has. Each file is H.264 video, 1280×720 or the camera's nearest, 30 frames a second,
AAC audio, mono, 48 kHz, with the mp4 index at the front. `started` is sent when the first
frames are written; the core's clock starts there. The picture keeps showing while recording.

## The microphone level (`level`)

    host → core   {"t":"level","db":-32.5}

About ten times a second while the microphone is open, for the meter. RMS in dBFS.

## Errors

    host → core   {"t":"error","what":"camera","text":"the camera is used by another app"}

`what` is `camera`, `microphone`, `record` or `picture`. The core shows `text` as it is.

## Links

A host that the system hands `poiesis://` links to writes each link, as it came, to the file
`command` in the app's state folder, and brings its window forward. The core reads that file,
follows what a link may do (`FORMAT.md`, Links) and removes it. The host never reads a link's
meaning, and never starts a recording because of one.

## Focus and ending

Focus comes through the terminal's own focus reporting, not the socket. When the core exits,
the host closes its window. When the socket closes, the core goes on without the host.

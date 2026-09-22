# What Poiesis is made of

Poiesis is AGPL-3.0 (see `LICENSE`). The vault format (`SCHEMA.md`, `docs/FORMAT.md`) and the
AI connection (`mcp.go`) are MIT (see `LICENSES/MIT.txt`), so that any tool may read and
write Poiesis vaults.

## Inside the Mac app

The Mac app carries copies of these programs so that nothing has to be installed. They
are unchanged except for being renamed or rewired to find their own libraries inside the
app. Their licences travel with them; here is the list.

| what | why | licence | source |
|---|---|---|---|
| Ghostty | the window: a terminal that draws real pixels under text, shipped renamed so macOS shows Poiesis's name | MIT, © Mitchell Hashimoto | github.com/ghostty-org/ghostty |
| ffmpeg, ffprobe | recording the camera, reading video | GPL-2.0-or-later as built by Homebrew (it includes x264) | ffmpeg.org |
| the libraries ffmpeg loads | codecs and formats | each its own: x264 (GPL), libvpx (BSD), opus (BSD), lame (LGPL), and others; the exact files are in `Poiesis.app/Contents/Frameworks/lib` | |
| whisper.cpp, ggml | speech to text | MIT, © Georgi Gerganov | github.com/ggml-org/whisper.cpp |
| Ollama | runs the local AI model | MIT | github.com/ollama/ollama |

## Fetched on first use, into the app's own folder

| what | licence | source |
|---|---|---|
| Whisper large-v3-turbo (ggml, q5_0) | MIT (OpenAI) | huggingface.co/ggerganov/whisper.cpp |
| Silero VAD | MIT | huggingface.co/ggml-org/whisper-vad |
| KB-Whisper large (Swedish, optional) | Apache-2.0 (KBLab) | huggingface.co/KBLab/kb-whisper-large |
| Qwen 3.5 4B (through Ollama) | Apache-2.0 | ollama.com/library |

## Go libraries

Bubble Tea, Bubbles, Lipgloss and x/ansi (MIT, Charm); the Model Context Protocol Go SDK
(MIT); the Anthropic Go SDK (MIT); fyne.io/systray (BSD-3, for the tray on Windows and
Linux); godbus (BSD-2); golang.org/x/sys (BSD-3); yaml.v3 (MIT and Apache-2.0).

## Colours and names

The themes and looks are Poiesis's own. The wordmark and the icon are Poiesis's own.

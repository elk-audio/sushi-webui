# sushi-webui

Responsive web control surface for the [Sushi](https://github.com/elk-audio/sushi) plugin host: a Mixer → Track → Plugin workflow for desktop, tablet and phone, with live meters, buses and sends.

A Go backend talks to Sushi over gRPC and serves a Svelte frontend over HTTP and WebSocket from the same machine. The result is a single binary with no runtime dependencies.

## Quick start (macOS)

Prerequisites: Go ≥ 1.22, Node ≥ 20 (build time only), `protoc` (only to regenerate stubs), and a Sushi build with gRPC.

```sh
# 1. Start Sushi (CoreAudio; see docs/IMPLEMENTATION_NOTES.md for why not --dummy)
./dev/run-sushi.sh                 # SUSHI_DIR defaults to ../sushi/build-macos-universal/dist/sushi-1.4.0

# 2. Build and run the WebUI (in another terminal)
make                               # builds web/dist, then bin/sushi-webui with the UI embedded
./bin/sushi-webui --plugin-path ../sushi/build-macos-universal/dist/sushi-1.4.0

# 3. Open http://localhost:8080  (or http://<this-mac>.local:8080 from a phone on the same network)
```

> The default track "Audio 1" takes the Mac's default input (microphone) and plays it to the default output. Turn the volume down or mute Master if you're on speakers.

Frontend development with hot reload: `make dev`. This starts the backend with `--dev` and Vite on http://localhost:5173, which proxies `/ws` to the backend.

## Backend flags

| Flag | Default | |
|------|---------|--|
| `--listen` | `:8080` | HTTP/WebSocket address |
| `--sushi` | `localhost:51051` | Sushi gRPC address |
| `--data-dir` | `~/.config/sushi-webui` | projects (`projects/*.sushiproj`), current metadata, `plugins.json` |
| `--plugin-list` | built-in list | categorized plugin list (vendor → category → plugin, sushi-gui `installed_plugins.json` format). External plugins not found on this machine are dropped at startup |
| `--plugin-path` | — | extra folder where the listed VST2/VST3/LV2 plugins are looked for (repeatable) |
| `--audio-dir` | `~/Music` | base folder for the Wav Streamer file picker (`*.wav`, up to 3 levels deep) |
| `--web-dir` | — | serve the frontend from disk instead of the embedded build |
| `--dev` | off | accept WebSocket connections from any origin (Vite proxy) |
| `--log-level` | `info` | `debug` shows every failed parameter write |

## Layout

```
cmd/sushi-webui/      entry point
internal/sushi/       gRPC client helpers (async command waits, name lookups)
internal/sushirpc/    generated stubs (make proto)
internal/core/        state mirror, Sushi sync, meters, mute/solo, sends, operations, projects
internal/catalog/     plugin catalog: categorized plugin list (embedded plugins.json) filtered by what is installed
internal/hub/         WebSocket fan-out
web/                  Svelte 5 + TypeScript frontend (Vite)
docs/                 ARCHITECTURE, UX_DESIGN, PROTOCOL, IMPLEMENTATION_NOTES
mockups/index.html    the approved interactive UX sketch
dev/                  Sushi launcher and config for development
```

## Elk Audio OS

`make arm64` builds a static `bin/sushi-webui-linux-arm64`. Deployment (systemd unit, `/udata` install) is specified in `docs/ARCHITECTURE.md` §8 but not yet tested on a device.

# sushi-webui

Responsive web control surface for the [Sushi](https://github.com/elk-audio/sushi) plugin host.

It offers a standard Mixer → Track → Plugin workflow for desktop, tablet and phone, with live meters, buses and sends.

A Go backend talks to Sushi over gRPC and serves a Svelte frontend over HTTP and WebSocket from the same machine. The result is a single binary with no runtime dependencies.

## Quick start (macOS)

You need Go 1.25 or newer, Node 20 or newer (only to build the frontend), and a Sushi 1.4 build with gRPC enabled. `protoc` is only needed to regenerate the gRPC stubs.

Point `SUSHI_DIR` at the folder that contains the `sushi` binary and your plugins:

    $ export SUSHI_DIR=/path/to/sushi/dist/sushi-1.4.0

Start Sushi with Core Audio in one terminal:

    $ ./dev/run-sushi.sh

Build and run the WebUI in another:

    $ make
    $ ./bin/sushi-webui --plugin-path $SUSHI_DIR

Then open http://localhost:8081. From a phone on the same network, use `http://<hostname>.local:8081`.

`run-sushi.sh` uses Core Audio instead of `--dummy`, because the dummy frontend crashes Sushi on graph edits. See `docs/IMPLEMENTATION_NOTES.md`.

The default config routes the Mac's default input (the microphone) to the default output. Lower the volume or mute Master if you are on speakers.

To run both processes with one command, use `./dev/run-all.sh`. It takes the same `SUSHI_DIR`, plus optional `AUDIO_DIR`, `LISTEN` and `MIDI_DEVICE`.

For frontend development with hot reload, run `make dev`. It starts the backend with `--dev` and Vite on http://localhost:5173, which proxies `/ws` to the backend.

## Backend flags

| Flag | Default | |
|------|---------|--|
| `--listen` | `:8081` | HTTP/WebSocket address |
| `--sushi` | `localhost:51051` | Sushi gRPC address |
| `--data-dir` | `~/.config/sushi-webui` | projects (`projects/*.sushiproj`), current metadata, `plugins.json` |
| `--plugin-list` | built-in list | categorized plugin list (vendor → category → plugin, sushi-gui `installed_plugins.json` format). Plugins that are not installed are dropped at startup |
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

## License

sushi-webui is licensed under the [GNU Affero General Public License v3](LICENSE).

---
Copyright 2026 Elk Audio AB, Stockholm, Sweden.

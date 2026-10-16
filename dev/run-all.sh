#!/bin/sh
# Start Sushi (dev/run-sushi.sh, with MIDI auto-mapping) and the backend together; Ctrl-C stops both.
# Env: AUDIO_DIR (Wav Streamer folder, default ~/audio/loops), LISTEN (default :8080), SUSHI_DIR,
# MIDI_DEVICE. Extra arguments go to sushi-webui. Builds bin/sushi-webui first if it is missing.
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(dirname "$HERE")
AUDIO_DIR=${AUDIO_DIR:-$HOME/audio/loops}
LISTEN=${LISTEN:-:8080}
SUSHI_DIR=${SUSHI_DIR:-$ROOT/../sushi/build-macos-universal/dist/sushi-1.4.0}
export SUSHI_DIR

[ -x "$ROOT/bin/sushi-webui" ] || make -C "$ROOT" || exit 1
mkdir -p "$AUDIO_DIR"

"$HERE/run-sushi.sh" &
SUSHI=$!
trap 'kill $SUSHI $WEBUI 2>/dev/null; wait' EXIT INT TERM

# The backend reconnects by itself, but waiting avoids a burst of connection errors at startup.
for _ in $(seq 1 50); do
    nc -z localhost 51051 2>/dev/null && break
    kill -0 $SUSHI 2>/dev/null || { echo "Sushi exited" >&2; exit 1; }
    sleep 0.2
done

"$ROOT/bin/sushi-webui" --listen "$LISTEN" --audio-dir "$AUDIO_DIR" --plugin-path "$SUSHI_DIR" "$@" &
WEBUI=$!
wait $WEBUI

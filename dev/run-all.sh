#!/bin/sh
# Start Sushi (dev/run-sushi.sh, with MIDI auto-mapping) and the backend together; Ctrl-C stops both.
# Env: SUSHI_DIR (required, folder containing the sushi binary), AUDIO_DIR (Wav Streamer folder,
# default: the backend's own default), LISTEN (default :8081), MIDI_DEVICE. Extra arguments go to sushi-webui. Builds bin/sushi-webui first if it is missing.
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(dirname "$HERE")
LISTEN=${LISTEN:-:8081}
[ -n "$SUSHI_DIR" ] || { echo "Set SUSHI_DIR to the folder that contains the sushi binary" >&2; exit 1; }
export SUSHI_DIR

[ -x "$ROOT/bin/sushi-webui" ] || make -C "$ROOT" || exit 1

"$HERE/run-sushi.sh" &
SUSHI=$!
trap 'kill $SUSHI $WEBUI 2>/dev/null; wait' EXIT INT TERM

# The backend reconnects by itself, but waiting avoids a burst of connection errors at startup.
for _ in $(seq 1 50); do
    nc -z localhost 51051 2>/dev/null && break
    kill -0 $SUSHI 2>/dev/null || { echo "Sushi exited" >&2; exit 1; }
    sleep 0.2
done

"$ROOT/bin/sushi-webui" --listen "$LISTEN" ${AUDIO_DIR:+--audio-dir "$AUDIO_DIR"} --plugin-path "$SUSHI_DIR" "$@" &
WEBUI=$!
wait $WEBUI

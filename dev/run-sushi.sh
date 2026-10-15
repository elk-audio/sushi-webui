#!/bin/sh
# Start Sushi for WebUI development on macOS.
# Uses CoreAudio: Sushi's --dummy frontend doesn't run the engine in realtime mode, so graph edits
# race with its processing thread and crash Sushi (see docs/IMPLEMENTATION_NOTES.md).
SUSHI_DIR=${SUSHI_DIR:-../sushi/build-macos-universal/dist/sushi-1.4.0}
HERE=$(cd "$(dirname "$0")" && pwd)
cd "$SUSHI_DIR" || { echo "Sushi not found in $SUSHI_DIR (set SUSHI_DIR)"; exit 1; }
exec ./sushi --coreaudio -c "$HERE/default.json" --grpc-address=localhost:51051 "$@"

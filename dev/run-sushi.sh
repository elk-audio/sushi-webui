#!/bin/sh
# Start Sushi for WebUI development on macOS.
# Uses CoreAudio: Sushi's --dummy frontend doesn't run the engine in realtime mode, so graph edits
# race with its processing thread and crash Sushi (see docs/IMPLEMENTATION_NOTES.md).
#
# MIDI: the first real controller (not IAC/loopback/"through" ports, nor a second "InControl"/"DAW"
# port of the same device) is mapped to Sushi MIDI port 0 through rt_midi_input_mappings in a
# generated copy of default.json. MIDI_DEVICE=<index> forces a CoreMIDI source; MIDI_DEVICE=none
# disables it. On Linux the connection is made with aconnect instead (not implemented yet).
#
# Env: SUSHI_DIR (required): folder containing the sushi binary, e.g. dist/sushi-1.4.0 of a Sushi build.
[ -n "$SUSHI_DIR" ] || { echo "Set SUSHI_DIR to the folder that contains the sushi binary" >&2; exit 1; }
HERE=$(cd "$(dirname "$0")" && pwd)
CONFIG=$HERE/default.json

pick_midi_device() {
    [ "$MIDI_DEVICE" = none ] && return
    [ -n "$MIDI_DEVICE" ] && { echo "$MIDI_DEVICE"; return; }
    BIN=${TMPDIR:-/tmp}/sushi-webui-midi-list
    if [ ! -x "$BIN" ] || [ "$HERE/midi-list.swift" -nt "$BIN" ]; then
        swiftc -O "$HERE/midi-list.swift" -o "$BIN" 2>/dev/null || { echo "can't build the MIDI lister (needs swiftc)" >&2; return; }
    fi
    "$BIN" | tee /dev/stderr | grep -v -i -E ' (IAC|.*(Bus [0-9]|Through|loopback|virtual|Network)|.*(InControl|DAW))' | head -n 1 | cut -d' ' -f1
}

case $(uname) in
Darwin)
    echo "MIDI sources:" >&2
    DEV=$(pick_midi_device)
    if [ -n "$DEV" ]; then
        echo "-> using MIDI source $DEV as Sushi MIDI port 0" >&2
        CONFIG=$(mktemp -t sushi-webui-config)
        python3 -I - "$HERE/default.json" "$CONFIG" "$DEV" <<'PY'
import json, sys
src, dst, dev = sys.argv[1], sys.argv[2], int(sys.argv[3])
cfg = json.load(open(src))
cfg.setdefault("host_config", {})["rt_midi_input_mappings"] = [
    {"rt_midi_device": dev, "sushi_midi_port": 0, "virtual_port": False}]
json.dump(cfg, open(dst, "w"), indent=2)
PY
    else
        echo "-> no real MIDI controller found" >&2
    fi
    ;;
*) echo "MIDI auto-connect on Linux is not implemented yet (use aconnect)" >&2 ;;
esac

cd "$SUSHI_DIR" || { echo "Sushi not found in $SUSHI_DIR (check SUSHI_DIR)" >&2; exit 1; }
exec ./sushi --coreaudio -c "$CONFIG" --grpc-address=localhost:51051 "$@"

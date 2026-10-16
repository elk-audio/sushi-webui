# Implementation notes, first implementation (2026-10-15)

Where v1 of the code differs from ARCHITECTURE.md, and what testing against Sushi 1.4.0 on macOS turned up.

## Simplifications compared with the architecture
- **Protocol:** structural changes resend the full `state` (debounced), instead of fine-grained deltas. Every client receives every parameter update; per-view subscriptions are not implemented yet. See `docs/PROTOCOL.md`.
- **Graph sync:** after any track or processor notification, the backend re-reads the graph. Parameter info is fetched only for processors it hasn't seen before; values stay current through notifications.
- **Waiting for creations:** Sushi answers create calls with `ASYNC_RESPONSE`. The backend waits for the matching `SubscribeToAsyncCommandUpdates` event, then polls `GetTrackId`/`GetProcessorId` by name for up to 2 s.
- **Unique names:** Sushi needs globally unique processor names.
  - Hidden processors get a random suffix: `__meter__Guitar#a1b2`, `__return__Reverb#…`, `__send__Guitar__Reverb#…`.
  - User plugins are named after the plugin, with " 2", " 3" and so on added for duplicates.
- **Renaming tracks:** Sushi has no rename RPC, so `renameTrack` stores a display label in the metadata (`TrackMeta.Label`, saved with projects). The Sushi-side name stays the key for metadata and Sushi calls; the state's `name` is the label. Input and Master can't be renamed.
- **Stale metadata:** on every connection the backend compares Sushi's tracks with the names in `current.meta.json`. If they differ (Sushi restarted from its default config), the metadata, including the project name, is reset to an untitled project.
- **Bypass and program** changes have no Sushi notification, so the backend updates its mirror after a successful call.
- **Plugin identity:** for processors created outside the WebUI, uid/type/path come from one `SaveSession` call. The role is then looked up in the catalog.
- **Catalog:** a predefined categorized list (`internal/catalog/plugins.json`, vendor → category → plugin, same format as sushi-gui's `installed_plugins.json`; `--plugin-list` replaces it).
  - Internal plugins are always offered. LV2, VST2 and VST3 entries are kept only if found at startup: LV2 by URI in a minimal line-based Turtle scan, VST2/VST3 by file name in the plugin folders.
  - Roles default from the category (`Generator` → instrument, `MIDI & Control` → MIDI effect, else audio effect); an item can set `"role"`.
  - The embedded list differs from the sushi-gui original: `BW Wha` uses `sushi.brickworks.wah`, `StepSequencer` uses `sushi.testing.step_sequencer`, PeakMeter and the CV plugins are removed, Sample Delay and Stereo Mixer are added.
- **Wav Streamer:** the file picker lists `*.wav` under `--audio-dir` (3 levels). `setWavFile` sets the `file` property and turns `playing` and `loop` on; paths outside the folder are refused.
- **MIDI input** is mapped by `dev/run-sushi.sh` through `rt_midi_input_mappings` (first real CoreMIDI source). The backend doesn't configure it.

## Workarounds for Sushi behaviour
1. **Mono audio tracks are created as 2-channel tracks**, with the one engine input connected to both channels (dual-mono).
   - Why: in Sushi 1.4.0, adding a stereo-only VST3 (e.g. `mda Combo`) to a 1-channel track logs "Failed to set a valid channel arrangement", and the plugin then segfaults in `doProcessing`.
   - Crash report: `ComboProcessor::doProcessing` ← `Vst3xWrapper::process_audio` ← `Track::_process_plugins`.
   - Side effect: pan acts as a balance control on mono tracks.
2. **Don't use `--dummy` for interactive testing.**
   - `OfflineFrontend::_process_dummy` runs a processing thread but never calls `engine->enable_realtime(true)`.
   - As a result, `AudioEngine::delete_track` and the other graph edits take the non-realtime path and modify the graph directly while the dummy thread renders it.
   - Two crashes were observed this way:
     - a pure virtual call in `Track::render()` right after "Destroyed processor (Reverb)";
     - heap corruption detected by malloc inside `SessionController::_save_engine_state()`.
   - With `--coreaudio` the same full test sequence passes repeatedly. **Suggested upstream fix:** have the dummy frontend enable realtime mode while its worker thread runs.
3. **An empty track list is rejected** by the JSON schema ("Schema validation failure at /tracks"). `dev/default.json` therefore starts with one empty stereo track named `__placeholder__`, plus a `post_track` named "Master". Sushi accepts an empty track list at runtime, so the backend deletes any `__placeholder__*` track when it connects (and again after loading a project), before the first state is sent. Any Sushi JSON config can use the same trick.

## Verified (automated, against Sushi 1.4.0 with CoreAudio)
These were checked with a scripted WebSocket client against the real backend:
- **Tracks and plugins:**
  - add audio, MIDI (with instrument) and bus (with first effect) tracks;
  - add internal plugins and a VST3 plugin, each placed in its section;
  - adding a second instrument is rejected;
  - reorder plugins; bypass a plugin.
- **Mixing:**
  - additive solo (buses solo-safe) and mute;
  - send levels and Pre/Post mode;
  - track gain and pan, and plugin parameter writes, with their notifications coming back.
- **Transport:** tempo and play.
- **Projects and removal:**
  - save a project; remove a send; remove a bus (its sends are removed first);
  - load the project again, with sends, mute and solo restored.
- **Meters:** binary frames carry live post-fader levels, e.g. microphone input through a send into the Reverb bus.
- **Frontend:**
  - `svelte-check` reports 0 errors.
  - Headless-Chrome screenshots of the Mixer, Track and Plugin views (desktop) and the Mixer and Track views (phone) rendered real backend state correctly.

## ZeroMQ transport (`--transport zmq`)
Sushi's ZeroMQ interface carries the same protobuf messages as gRPC, so `internal/sushi/zmqconn` implements `grpc.ClientConnInterface` over it and the generated clients, `sushi.Client` and the whole engine are shared. `--transport grpc` (default) or `zmq` is chosen at startup; only one is active.
- Control is REQ/REP on `ipc:///tmp/sushi_ipc.sock`, notifications are PUB/SUB on `ipc:///tmp/sushi_pub.sock` (`--zmq-control`, `--zmq-notify`). Unix sockets: same machine only, one Sushi instance.
- Wire ids (service, endpoint, notification code) are derived from the compiled proto descriptors; `zmqconn/wire_test.go` pins them to the numbers read from Sushi's `protoc-gen-zmq` output.
- Sushi's REP loop stops answering for good after one malformed request, and `CvGateController` dereferences a null pointer. The client only sends well-formed requests and refuses that service.
- Calls are serialised (one REQ socket). A timed-out call discards the socket and the next call opens a fresh one.
- Sushi's PUB socket has a send high-water mark of 20, so bursts (our meters are parameter updates) can be dropped on Sushi's side. Not seen in tests, to be watched on real hardware.
- A SUB socket doesn't notice Sushi going away, so a 2 s keepalive (`GetSushiVersion`) ends the streams after two misses; the engine's supervisor then reconnects as with gRPC.
- Subscriptions are global and permanent in Sushi; the parameter blocklist is ignored.
- Tests: `internal/sushi/fakesushi` is an in-memory Sushi served over bufconn gRPC or a fake ZeroMQ server that reproduces the quirks above. `internal/sushi/transport_test.go` runs the same checks over both; `internal/core/engine_test.go` runs the engine against both. `make test`.
- Not verified yet against a real Sushi (libzmq interop of the pure-Go `go-zeromq/zmq4`).

## Not yet verified
- Touch gestures (knob, fader, swipe, drag-to-reorder) on real devices; the gesture code has only been reviewed, not driven.
- Elk Audio OS / Raspberry Pi: the `make arm64` build isn't tested on a device, and there's no systemd unit yet.
- Large VST3 plugins (hundreds of parameters): the parameter grid isn't virtualized.

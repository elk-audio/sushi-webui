# sushi-webui WebSocket protocol, v1 (as implemented)

One WebSocket per browser at `GET /ws`. Text frames carry JSON objects with a `t` (type) field. Binary frames carry meters.

This document is the contract between `web/` (Svelte) and the Go backend. It is a simplified version of the protocol in ARCHITECTURE.md §5:
- Any structural change resends the full `state`. The state is small, at most a few hundred kB even with large plugins.
- Every client receives every parameter update.

## Server → client

```jsonc
{"t":"hello","proto":1,"client":"c3"}
{"t":"state","rev":12,"state":State}        // on connect, and (debounced ~50 ms) after any structural change
{"t":"params","u":[[procId,paramId,norm,"formatted text"], ...]}   // batched, flushed every ~20 ms
{"t":"transport","tempo":120.0,"playing":true}
{"t":"cpu","v":0.23}                         // engine main-thread average load, 0..1, ~1 Hz
{"t":"result","id":7,"ok":true}
{"t":"result","id":8,"ok":true,"data":{"id":12}}   // addTrack / addPlugin return the new track / processor id
{"t":"result","id":7,"ok":false,"error":"Sushi couldn't create the track: ERROR"}
{"t":"conn","sushi":false}                   // backend lost Sushi; a new state follows on reconnect
```

### State
```ts
type Kind = 'audio' | 'midi' | 'bus' | 'pre' | 'post';
type Role = 'audio_fx' | 'midi_fx' | 'instrument';

interface State {
  sushi: { connected: boolean; version: string; inputs: number; outputs: number; midiInputs: number; samplerate: number; defaultInput: number; defaultOutput: number };  // default*: first engine channel (0-based) new tracks use
  project: { name: string; dirty: boolean; list: string[] };   // list = saved project names
  ui: { advanced: boolean };
  transport: { tempo: number; playing: boolean };
  cpu: number;
  tracks: Track[];            // display order: audio/midi tracks first, then buses
  pre: Track | null;          // Sushi PRE track ("Input"), if it exists
  master: Track | null;       // Sushi POST track ("Master")
  catalog: CatalogEntry[];
}
interface Track {
  id: number; name: string; kind: Kind; color: string; channels: number;
  gain: Param | null;         // the track's "gain" parameter (dB −120..+24, linear in dB on the normalized scale)
  pan: Param | null;          // only when channels <= 2 and kind is not pre/post
  mute: boolean;              // the user's mute (not including mutes implied by solo)
  solo: boolean;
  input: string;              // display text, e.g. "In 1-2", "MIDI port 0 · Omni", "Sends", "In 1-8"
  output: string;             // display text, e.g. "Out 1-2"
  inChan: number;             // first engine input channel (0-based); -1 for MIDI/bus/none
  outChan: number;            // first engine output channel (0-based); -1 when unconnected
  inMono: boolean;            // audio track whose two channels both read engine input `inChan`
  chain: Processor[];         // user-visible processors only (no meter/send/return)
  sends: Send[];
  meter: number;              // meter slot in binary frames, -1 if none
}
interface Param {
  id: number; name: string; label: string; unit: string;
  type: 'float' | 'int' | 'bool'; automatable: boolean;
  min: number; max: number;   // domain range
  n: number;                  // current normalized value 0..1
  txt: string;                // Sushi's formatted value
}
interface Processor {
  id: number; name: string; label: string;
  uid: string; type: 'internal' | 'vst2' | 'vst3' | 'lv2' | 'unknown'; role: Role;
  bypass: boolean;
  program: number;            // -1 if no programs
  programs: string[];
  params: Param[];            // all parameters (automatable and outputs)
  props: { id: number; name: string; label: string; value: string }[];
  macros: number[];           // param ids pinned to the Track-mode card
}
interface Send { bus: number; busName: string; db: number; post: boolean }
interface CatalogEntry { id: string; name: string; vendor: string /* top level, e.g. "Brickworks" */; category: string /* second level, e.g. "Reverb" */; uid: string; path: string; type: 'internal' | 'vst2' | 'vst3' | 'lv2'; role: Role }
```

## Client → server

```jsonc
{"t":"set","p":procId,"k":paramId,"n":0.42}        // any parameter; tracks use the track id as procId (gain, pan)
{"t":"send","track":5,"bus":9,"db":-12.5}           // send level, coalesced like "set"
{"t":"cmd","id":1,"op":"<op>","args":{...}}          // answered with "result"
```

| op | args |
|----|------|
| `addTrack` | `{kind:'audio'\|'midi'\|'bus', name, channels?:1\|2, input?:int (first engine input, 0-based), output?:int (first engine output), midiPort?:int, midiChannel?:int (0 = omni, 1..16), instrument?:catalogId, fx?:catalogId, sendFrom?:trackId}` |
| `setRouting` | `{track, input?:int, output?:int}`: first engine channel (0-based). Output must be an even channel with a pair available. Input is a pair start for stereo tracks, any single channel for dual-mono ones (`inMono`); audio tracks only. Disconnects the old channels, then connects the new ones. Refused for Input/Master |
| `removeTrack` | `{track}` |
| `renameTrack` | `{track, name}` (1–40 characters, unique among tracks; not for Input/Master) |
| `setColor` | `{track, color}` |
| `setMute` / `setSolo` | `{track, on}` |
| `addPlugin` | `{track, catalogId, section: Role}` |
| `removePlugin` | `{proc}`: only visible plugins; hidden processors (meter, sends, return) are refused. Also drops the plugin's sidecar metadata |
| `movePlugin` | `{proc, index}`: target index in the track's visible chain (the chain without `proc`; past the end means last). Refused when it would leave the section order MIDI FX → instrument → audio FX, e.g. a generator after an effect |
| `setBypass` | `{proc, on}` |
| `setProgram` | `{proc, program}` |
| `setMacros` | `{proc, params:int[]}` |
| `setProperty` | `{proc, prop, value}` |
| `listWavs` | `{}` → result data `{files: string[]}`: `*.wav` files under the backend's `--audio-dir`, as relative paths |
| `setWavFile` | `{proc, value}` (a path from `listWavs`) on a Wav Streamer: sets its `file` property and turns `playing` and `loop` on |
| `addSend` / `removeSend` | `{track, bus}` |
| `setSendMode` | `{track, bus, post}` |
| `setAdvanced` | `{on}` |
| `transport` | `{playing?:bool, tempo?:number}` |
| `saveProject` | `{name?}` |
| `loadProject` | `{name}` |

## Binary meter frame
```
u8  type = 1
u16 count (LE)
count × { u16 slot (LE) | u8 levelL | u8 levelR | u8 flags }
```
- **Level byte:** 0..255 maps to −60..+6 dB, so `db = -60 + b * 66 / 255`. A value of 0 means −60 dB or below.
- **Mono tracks:** levelR = levelL.
- **Flags:** bit0 = clip left, bit1 = clip right since the last frame. The client latches clips until the user clears them.
- **Levels are post-fader:** the backend adds the track's gain/pan law and mute. Frames are sent at most every 33 ms and only for slots that changed.

## Echo handling
The client ignores `params` entries for a parameter while the user is dragging it. When the gesture ends, the client applies the last value received.

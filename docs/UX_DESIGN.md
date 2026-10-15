# Sushi WebUI — UX Design

Status: **draft for review** · Companion to `ARCHITECTURE.md` · Interactive sketch: `mockups/index.html`

## 1. Who and why

| Persona | Device | Typical task |
|---------|--------|--------------|
| **Live player** (guitarist, keyboardist on stage with an Elk Pi board) | phone on a mic-stand clamp, tablet | switch presets, tweak drive/reverb amount, mute a track, check levels/clipping |
| **Sound designer at the desk** | laptop/desktop | build a rig: add tracks, choose plugins, order the chain, set routing, save project |
| **Installer/integrator** | laptop, then phone in the field | load a project, verify signal flow and meters, small trims |

Design principles
1. **Levels are always visible.** Every view keeps a meter for the track in focus; clipping is impossible to miss.
2. **One thumb, one control.** On phone every control is ≥ 44 px, and drags never fight page scroll.
3. **Three depths, same gestures.** Mixer → Track → Plugin; tap to go deeper, back/breadcrumb/swipe to go up.
4. **Sushi is the truth.** The UI reflects confirmed engine state; pending operations are visible, never faked.
5. **Stage-safe.** Destructive actions (delete track/plugin, load project) need confirmation; no accidental double-tap disasters.

## 2. Information architecture

```
Project ─┬─ Mixer (all tracks + Master)
         │    └─ Track "Guitar" (chain: Gate → Drive → Cab → Reverb)
         │          └─ Plugin "Drive" (all parameters, programs)
         ├─ Add track sheet (Audio | MIDI)
         ├─ Plugin browser sheet (filtered by section)
         └─ Project menu (save, save as, load, new)
```

Global chrome (all sizes):
- **Top bar**: project name (with unsaved dot), breadcrumb (desktop/tablet) or back + title (phone), tempo, play/stop, CPU %, connection dot.
- Connection states: green = live, amber = reconnecting (UI dims, controls disabled), red = Sushi engine not running.

Routes: `#/mixer`, `#/track/Guitar`, `#/track/Guitar/plugin/Drive` — bookmarkable (e.g. a phone home-screen shortcut straight into one plugin).

## 3. Breakpoints and layout grid

| Class | Width | Mixer | Track | Plugin |
|-------|-------|-------|-------|--------|
| Desktop | ≥ 1024 | vertical strips, ~88 px each, horizontal scroll, Master pinned right | horizontal chain of plugin cards in sections + right inspector (track fader/meter/routing) | parameter grid (knobs, 6–8 per row) + left sidebar with programs |
| Tablet | 600–1023 | strips ~76 px | chain cards wrap; inspector becomes bottom dock | 4–5 knobs per row, programs in header dropdown |
| Phone portrait | < 600 | **list of track rows** with horizontal fader + meter | **vertical chain** of collapsible cards, track fader docked at bottom | 2-col big knobs; tap knob → Focus slider |
| Phone landscape | < 600 height | compact strips (64 px), no chain chips | as tablet | 3-col knobs |

## 4. Mixer mode

### Desktop / tablet
```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ● Live Rig*    Mixer                         ♩ 120.0  ▶ ■   CPU 23%   ● live │
├──────────────────────────────────────────────────────────────────────────────┤
│ ┌Guitar──┐┌Vocal───┐┌Synth───┐┌Drums───┐┌ ＋  ┐                 ┃┌MASTER──┐ │
│ │▮ audio ││▮ audio ││▮ MIDI  ││▮ audio ││     │                 ┃│        │ │
│ │Gate    ││Comp    ││Arp     ││Comp    ││ Add │                 ┃│        │ │
│ │Drive   ││EQ      ││Synth   ││        ││track│                 ┃│        │ │
│ │Cab +2  ││Reverb  ││Chorus  ││        ││     │                 ┃│        │ │
│ │  (◠)   ││  (◠)   ││  (◠)   ││  (◠)   │└─────┘                 ┃│        │ │
│ │ ║▌▌    ││ ║▌▌    ││ ║▌▌    ││ ║▌▌    │                        ┃│ ║▌▌    │ │
│ │ ╫▌▌    ││ ║▌▌    ││ ╫▌▌    ││ ║▌▌    │                        ┃│ ╫▌▌    │ │
│ │ ║▌▌    ││ ╫▌▌    ││ ║▌▌    ││ ╫▌▌    │                        ┃│ ║▌▌    │ │
│ │-3.5 dB ││ 0.0 dB ││-8.0 dB ││-1.2 dB │                        ┃│-0.5 dB │ │
│ │ [M][S] ││ [M][S] ││ [M][S] ││ [M][S] │                        ┃│  [M]   │ │
│ └────────┘└────────┘└────────┘└────────┘                        ┃└────────┘ │
└──────────────────────────────────────────────────────────────────────────────┘
```
Strip anatomy (top → bottom): color tab + name (tap = open track; long-press = rename/color/delete) · kind badge (AUDIO / MIDI + input "In 1-2" or "MIDI ch 1") · chain summary chips (max 3 + "+n"; tap chip = open that plugin directly) · pan knob · fader with meter beside it (meter shows L/R, peak-hold tick, clip LED on top) · dB readout (tap = type value on desktop) · Mute / Solo.

Master strip (Sushi post track) pinned on the right with a stronger frame; no solo, no chain chips (its plugins are reachable by tapping its name).

### Phone portrait
```
┌──────────────────────────────┐
│ Live Rig*         ▶  23% ●   │
├──────────────────────────────┤
│▮ Guitar        AUDIO   M  S  │
│  ▬▬▬▬▬▬▬▬▬▬▬▬░░░░  -3.5 dB   │  ← horizontal fader, meter drawn inside the track
│▮ Vocal         AUDIO   M  S  │
│  ▬▬▬▬▬▬▬▬▬▬▬▬▬▬░░   0.0 dB   │
│▮ Synth         MIDI    M  S  │
│  ▬▬▬▬▬▬▬▬░░░░░░░░  -8.0 dB   │
│  ＋ Add track                │
├──────────────────────────────┤
│ MASTER ▬▬▬▬▬▬▬▬▬▬▬▬░ -0.5 dB │  ← master docked at bottom
├──────────────────────────────┤
│  Mixer     Tracks    Project │  ← bottom tab bar
└──────────────────────────────┘
```
- Row tap (outside controls) → Track mode. Fader: horizontal relative drag; the meter is rendered *behind* the fader thumb so one row shows both.
- Pan lives in Track mode on phone (rarely changed live).
- Swipe a row left → quick actions (Delete, Duplicate later).

### Add-track sheet
1. Choice: **Audio track** (mic/guitar/line input) or **MIDI track** (synth/sampler from a keyboard).
2. Audio: name (auto "Audio 3"), mono/stereo, input channels (picker shows engine channel count from `GetInputAudioChannelCount`), output pair (default 1-2).
3. MIDI: name, MIDI input port + channel (Omni default), **instrument** (picker of `instrument` role plugins; can be "none for now"), output pair.
4. Create → strip appears with spinner until Sushi confirms; then the app navigates into the new Track view (desktop: stays in mixer, highlights new strip).

### Empty project
A single centered call to action: "Add your first track" with the two choices visible as large cards (Audio / MIDI). Master strip already visible.

## 5. Track mode

### Desktop
```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Live Rig* › Mixer › Synth                        ♩120  ▶ ■   CPU 23%  ● live │
├──────────────────────────────────────────────────────────────────────┬───────┤
│ MIDI IN  ┊ MIDI FX            ┊ INSTRUMENT         ┊ AUDIO FX          │ Synth │
│ port 0   ┊ ┌Arpeggiator─┐ ＋  ┊ ┌Simple Synth──┐   ┊ ┌Chorus──┐ ┌Rev┐ ＋│ (◠)pan│
│ ch Omni  ┊ │⏻  Prog: —  │     ┊ │⏻ Prog: Pad ▾ │   ┊ │⏻       │ │⏻  │  │ ║▌▌  │
│  ──▶     ┊ │(◠)(◠)(◠)   │     ┊ │(◠)(◠)(◠)(◠)  │   ┊ │(◠)(◠)  │ │(◠)│  │ ╫▌▌  │
│          ┊ │rate range  │     ┊ │cut res atk rel│  ┊ │rate dep│ │mix│  │ ║▌▌  │
│          ┊ └────────────┘     ┊ └──────────────┘   ┊ └────────┘ └───┘  │-8 dB  │
│          ┊                    ┊                    ┊           ──▶ OUT 1-2 [M][S]│
└──────────────────────────────────────────────────────────────────────┴───────┘
```
- Signal flows left → right; section headers are labels on a thin rail. Audio tracks show `AUDIO IN (1-2)` → `FX` → `OUT`.
- **Plugin card**: name (tap = Plugin mode), power/bypass toggle (bypassed card dims), program selector (if `program_count > 0`), 2–6 **macro knobs** (default: first automatable FLOAT params; editable via "Pin to card" in Plugin mode), drag handle for reorder.
- **＋ slots** at end of each section (and between cards on hover) open the plugin browser filtered by that section's role. On MIDI tracks the Instrument section accepts exactly one plugin; replacing it asks for confirmation.
- Reorder by dragging cards within their section; invalid drop targets are not highlighted.
- Card overflow menu: Replace…, Delete, Duplicate (later), Reset parameters (later).
- **Inspector** (right): track name/color, pan, fader + meter, M/S, routing (input/output channels, MIDI port/channel), delete track.

### Phone portrait
```
┌──────────────────────────────┐
│ ‹ Mixer      Synth     MIDI  │
├──────────────────────────────┤
│ MIDI IN · port 0 · Omni      │
│ ┌─ MIDI FX ────────────────┐ │
│ │⏻ Arpeggiator        ›  ≡ │ │
│ │  (◠) rate  (◠) range     │ │
│ └──────────────────────────┘ │
│ ┌─ INSTRUMENT ─────────────┐ │
│ │⏻ Simple Synth  Pad ▾ › ≡ │ │
│ │  (◠)cut (◠)res (◠)atk    │ │
│ └──────────────────────────┘ │
│ ┌─ AUDIO FX ───────────────┐ │
│ │⏻ Chorus             ›  ≡ │ │  collapsed cards show only the header row
│ │⏻ Reverb             ›  ≡ │ │
│ │       ＋ Add effect       │ │
│ └──────────────────────────┘ │
├──────────────────────────────┤
│ ▬▬▬▬▬▬▬▬░░░░ -8.0 dB  M  S  │  ← docked track fader+meter
└──────────────────────────────┘
```
- Cards collapse/expand by tapping the header area; first card of each section expanded by default.
- Reorder: long-press the ≡ handle, drag vertically.

## 6. Plugin mode

### Desktop
```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Live Rig* › Mixer › Synth › Simple Synth            ♩120 ▶ ■  CPU 23% ● live │
├───────────────┬──────────────────────────────────────────────────────────────┤
│ PROGRAMS      │ ⏻ Simple Synth      ◀ Warm Pad ▶      [search params…]  ║▌▌ │
│  Init         │ ─ Oscillator ─────────────────────────────────────────────── │
│ ▸ Warm Pad    │  (◠)     (◠)      (◠)      [ Saw ▾ ]                          │
│  Bright Lead  │  Pitch   Detune   PW       Waveform                           │
│  Pluck        │ ─ Filter ─────────────────────────────────────────────────── │
│  Bass         │  (◠)     (◠)      (◠)                                         │
│               │  Cutoff  Reso     Env amt                                     │
│ [Pin to card] │ ─ Envelope ───────────────────────────────────────────────── │
│               │  (◠)     (◠)      (◠)      (◠)                                │
│               │  Attack  Decay    Sustain  Release                            │
└───────────────┴──────────────────────────────────────────────────────────────┘
```
- Header: bypass, name, program prev/next + current name (tap = list), parameter search, a small track meter at the right.
- **Parameter grid**: FLOAT → knob, BOOL → toggle, INT → stepper (or dropdown if its `formatted_value` strings look enumerated, e.g. waveform), properties (strings) → text/file field. Each control shows label + `formatted_value` (Sushi's string, e.g. "1.2 kHz").
- **Grouping**: by common name prefix (`osc_`, `filter_`, `env_`) or by the plugin's natural ordering in chunks of 8 when no prefix exists.
- Non-automatable (output) params shown in a separate "Outputs" group as read-only values/mini meters.
- Large plugins (100+ params): virtualized grid + search; groups collapsed by default.
- "Pin to card": selection mode — tap knobs to choose the macros shown on the Track card.
- Desktop extras: mouse wheel on knobs, double-click a value to type it.

### Phone portrait
```
┌──────────────────────────────┐
│ ‹ Synth   Simple Synth    ⏻ │
│   ◀  Warm Pad  ▶       ║▌▌  │
├──────────────────────────────┤
│ OSCILLATOR                   │
│   ( ◠ )        ( ◠ )         │
│   Pitch 0 st   Detune 12 ct  │
│ FILTER                       │
│   ( ◠ )        ( ◠ )         │
│   Cutoff 1.2k  Reso 0.30     │
└──────────────────────────────┘
       tap a knob ⇒
┌──────────────────────────────┐
│  Cutoff                  ✕   │
│         1.20 kHz             │
│ ├────────────●──────────────┤│  ← Focus slider: full width, fine by default
│  − fine         coarse +     │
│  [ Reset ]                   │
└──────────────────────────────┘
```
- Knobs on phone are 72 px; dragging directly on them works, but a tap opens the **Focus slider** sheet for precise control with the thumb (horizontal travel is longer than any knob drag).

## 7. Widgets — interaction spec

| Widget | Size (min) | Gesture | Feedback | Reset |
|--------|-----------|---------|----------|-------|
| Knob | 44 px (desktop 40, phone 72 in plugin grid) | vertical drag, 200 px = full range; Shift / 2nd finger = ×0.1 fine; wheel | arc fill, value bubble during drag | double-tap → default |
| Pan knob | 36–44 px | as knob, centre detent ±2 % | "L 30 / C / R 30" | double-tap → C |
| Fader | 44 px wide thumb | relative drag; no jump on tap | dB readout; 0 dB tick; unity detent | double-tap → 0 dB |
| Phone row fader | full row width, 44 px tall | horizontal relative drag; must move 6 px horizontally before capture (vertical motion scrolls list) | meter rendered inside track | double-tap → 0 dB |
| Toggle (M/S/bypass) | 44×32 | tap | colored fill (M amber, S teal-blue, bypass dims card) | — |
| Program selector | — | tap arrows; tap name → searchable list | name updates when Sushi confirms | — |
| Meter | 6 px per channel (desktop), 4 px (phone) | tap clip LED to clear | green → amber (−12 dB) → red (−3 dB); peak tick holds 1.5 s; clip LED latches | — |

Fader law (display): position ↔ dB mapping piecewise: +24…0 dB top 25 %, 0…−20 dB 45 %, −20…−60 dB 25 %, −60…−120 dB last 5 %. The UI converts this position to dB, then to Sushi's normalized value using the param's min/max (Sushi's gain param is linear in dB over −120..+24).

Meter scale: −60 … +6 dB, tick marks at 0, −6, −12, −24, −48.

## 8. States and feedback
- **Pending operation**: e.g. adding a plugin shows a ghost card with a spinner in the target slot; on `result.ok=false` the ghost turns into an inline error ("Sushi couldn't load 'Dexed.vst3': NOT_FOUND") with Retry/Dismiss.
- **Reconnecting**: amber top bar, all controls disabled (50 % opacity), meters freeze at −∞.
- **Engine missing**: full-screen message "Sushi isn't running on this device" with the address being tried.
- **Destructive confirmation**: inline confirm in place (button turns into "Delete Guitar? Delete / Cancel"), no modal dialogs.
- **Clip**: clip LED latches red on the strip and the track row in phone mixer; tap to clear; Master clip also shows as a red dot on the top-bar project name so it's visible from any depth.
- **Unsaved changes**: dot after project name; Project menu offers Save.

## 9. Visual language
- Dark-first, low-glare for stage (light theme optional later).
- Neutral graphite surfaces with slight blue bias; one warm accent (amber) for interactive focus/selection; semantic meter colors (green/amber/red) kept separate from the accent.
- Track colors: 8-color palette assigned round-robin, user-changeable; used for strip tab, chain section rail, and phone row stripe.
- Type: condensed sans for labels (fits narrow strips), monospaced tabular figures for values.

## 10. Open questions for review
1. **Solo** semantics: exclusive (one at a time) or additive? Draft: additive, Alt/long-press = exclusive.
2. Should **Pre track** ("Input") be exposed in the mixer at all, or hidden in an "Advanced" area?
3. **Sends/returns** (`sushi.testing.send`/`return`): v1 hidden, or a simple "Send to FX bus" per strip?
4. **Snapshots** within a project (quick recall of a set of parameter values on stage) — v2?
5. **MIDI learn** (`ConnectCCToParameter`) from Plugin mode via long-press → "Learn MIDI CC" — v1.1 candidate.
6. Program change from a MIDI footswitch (`ConnectPCToProcessor`) configuration UI — v1.1.
7. Phone bottom tab bar: is a "Tracks" tab (quick switch between tracks) more useful than swipe-left/right between tracks in Track mode?

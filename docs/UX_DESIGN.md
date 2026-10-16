# Sushi WebUI — UX Design

Status: **draft v2, after first review** · Companion to `ARCHITECTURE.md` · Interactive sketch: `mockups/index.html`

Review decisions so far (2026-10-15):
- **Solo** is additive: several tracks can be soloed at once. Buses are solo-safe.
- **Advanced option** "Show Input and Output tracks" exposes Sushi's pre and post tracks. It's off by default.
- **Buses and sends** are a first-class but limited concept:
  - A bus is an FX-only track.
  - A track sends to a bus by name.
  - The send and return processors stay hidden.
- **Next/previous** controls are in Track mode on every device, plus swipe on touch.
- **No phone tab bar.** The Project menu opens from the project name in the top bar, which frees vertical space.
- **Track mode** hides the input/output blocks and the channel strip by default (behind "Mix & routing"). Mixing belongs to Mixer mode.
- **Phone mixer** rows include pan.
- **Later versions:** snapshots and MIDI learn.

## 1. Who and why

| Persona | Device | Typical task |
|---------|--------|--------------|
| **Live player** (guitarist, keyboardist on stage with an Elk Pi board) | phone on a mic-stand clamp, tablet | switch presets, tweak drive/reverb amount, mute a track, check levels/clipping |
| **Sound designer at the desk** | laptop/desktop | build a rig: add tracks, choose plugins, order the chain, set routing, save project |
| **Installer/integrator** | laptop, then phone in the field | load a project, verify signal flow and meters, small trims |

Design principles
1. **Levels are always visible.** Every view keeps a meter for the track in focus; clipping is impossible to miss.
2. **One thumb, one control.** On phone every control is ≥ 44 px, and drags never fight page scroll.
3. **Three depths, same gestures.** Mixer → Track → Plugin. Tap to go deeper, and use back, the breadcrumb or swipe to go up. At each depth, ‹ › move between siblings: tracks in Track mode, plugins in Plugin mode.
4. **Each mode does one job.** The Mixer handles levels, pan, mute/solo and sends. Track mode handles the plugin chain and sends. Plugin mode handles parameters and programs. Controls from another mode are hidden unless asked for.
5. **Sushi is the truth.** The UI reflects confirmed engine state; pending operations are visible, never faked.
6. **Hide the plumbing.** Meters, sends and returns are Sushi processors. The UI shows them as track features, never as plugins.
7. **Stage-safe.** Destructive actions (delete track/plugin, load project) need confirmation; no accidental double-tap disasters.

## 2. Information architecture

```
Project ─┬─ Mixer
         │    ├─ [Input]          (advanced: Sushi pre track)
         │    ├─ Tracks           (Audio | MIDI)
         │    │    └─ Track "Guitar": chain Gate → Drive → Cab, sends → Reverb, → Delay
         │    │          └─ Plugin "Drive": all parameters, programs
         │    ├─ Buses            (FX only, fed by sends)
         │    │    └─ Bus "Reverb": chain Dragonfly Room
         │    └─ Master           (Sushi post track; its chain only in advanced mode)
         ├─ Add sheet (Audio | MIDI | Bus)
         ├─ Plugin browser sheet (filtered by section)
         ├─ Add-send sheet (pick a bus, or create one)
         └─ Project menu (save, save as, open, new, Advanced: show Input/Output tracks)
```

Track kinds as the user sees them, and how each one is built in Sushi:

| UI kind | What it is | Sushi implementation (hidden processors in *italics*) |
|---------|-----------|-------------------------------------------------------|
| Audio track | engine input(s) → FX → out | REGULAR track, audio input connections, chain, *send×n*, *peakmeter* |
| MIDI track | MIDI port/channel → MIDI FX → instrument → FX → out | REGULAR track, keyboard connection, chain, *send×n*, *peakmeter* |
| Bus | sends → FX → out | REGULAR track, no input connections, *return* first, chain, *peakmeter* |
| Input (advanced) | FX on all engine inputs before any track | PRE track, chain, *peakmeter* |
| Master / Output | master fader; in advanced mode also FX on all outputs | POST track (gain/mute), chain, *peakmeter* |

Global chrome on every screen size:
- **Top bar:**
  - project name (a dot marks unsaved changes; a red dot means master clipped). Tapping it opens the Project menu on every device;
  - a breadcrumb on desktop and tablet, or a back button on phone;
  - tempo, play/stop and CPU %;
  - a connection dot.
- **Connection dot states:**
  - green: live;
  - amber: reconnecting (the UI dims and controls are disabled);
  - red: Sushi engine not running.

Routes are `#/mixer`, `#/track/Guitar` and `#/track/Guitar/plugin/Drive`. They can be bookmarked, for example as a phone home-screen shortcut that opens one plugin directly.

## 3. Breakpoints and layout grid

| Class | Width | Mixer | Track | Plugin |
|-------|-------|-------|-------|--------|
| Desktop | ≥ 1024 | vertical strips ~92 px, horizontal scroll; groups: [Input] · Tracks · Buses; Master pinned right | header (‹ name › · meter · Mix & routing) + horizontal chain of cards in sections + Sends | parameter grid + left programs sidebar |
| Tablet | 600–1023 | strips ~80 px | as desktop, cards scroll horizontally | 4–5 knobs per row, programs in header |
| Phone portrait | < 600 | **list rows**: name · kind · M S / pan · horizontal fader+meter · dB | header + **vertical chain** of collapsible cards; swipe between tracks | 2-col big knobs; tap knob → Focus slider |
| Phone landscape | height < 520 | compact strips: plugin chips and send knobs hidden | as tablet (full width for the chain; no side panels by default) | 3-col knobs |

## 4. Mixer mode

### Desktop / tablet
```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ ● Live Rig*  /  Mixer                                  ♩ 120.0  ▶ ■   CPU 23%   ●    │
├──────────────────────────────────────────────────────────────────────────────────────┤
│[Input]┃T┌Guitar─┐┌Vocal──┐┌Synth──┐┌  ＋   ┐┃B┌Reverb─┐┌Delay──┐            ┌Master─┐│
│ adv.  ┃R│AUDIO  ││AUDIO  ││MIDI   ││ Add   │┃U│BUS    ││BUS    │            │OUT 1-8││
│ only  ┃A│Gate   ││Comp   ││Arp    ││ track ││S│Dragon…││Comb D…│            │       ││
│       ┃C│Drive  ││EQ     ││Synth  ││ or bus│┃E│       ││EQ     │            │       ││
│       ┃K│Cab    ││       ││Chorus │└───────┘┃S│       ││       │            │       ││
│       ┃S│(·)(·) ││(·)    ││(·)(·) │         ┃ │       ││       │            │       ││  ← send knobs:
│       ┃ │Rev Dly││Rev    ││Rev Dly│         ┃ │       ││       │            │       ││    → Reverb, → Delay
│       ┃ │  (◠)  ││  (◠)  ││  (◠)  │         ┃ │  (◠)  ││  (◠)  │            │       ││  ← pan
│       ┃ │ ║▌▌   ││ ║▌▌   ││ ║▌▌   │         ┃ │ ║▌▌   ││ ║▌▌   │            │ ║▌▌   ││
│       ┃ │ ╫▌▌   ││ ╫▌▌   ││ ╫▌▌   │         ┃ │ ╫▌▌   ││ ╫▌▌   │            │ ╫▌▌   ││
│       ┃ │-3.5 dB││ 0.0 dB││-8.0 dB│         ┃ │-2.0 dB││-5.0 dB│            │-0.5 dB││
│       ┃ │ [M][S]││ [M][S]││ [M][S]│         ┃ │ [M][S]││ [M][S]│            │  [M]  ││
└──────────────────────────────────────────────────────────────────────────────────────┘
```

**Strip anatomy**, top to bottom:
- **Colour tab and name:** tap to open the track; long-press to rename, recolour or delete.
- **Delete:** a small trash button on every strip and phone row except Input and Master. It asks inline "Delete X and its N plugins?" (a bus also mentions the sends into it). Track mode has the same button in its header, and the desktop Mix panel keeps its Delete button.
- **Kind badge:** AUDIO / MIDI / BUS, plus the input ("In 1", "MIDI port 0 · Omni") or, for a bus, its number of sends.
- **Chain summary chips:** at most 3 plus "+n". Tapping a chip opens that plugin directly.
- **Send knobs:** at most 2 mini knobs, each labelled with the bus name, plus "+n sends". This row is always reserved so faders line up across strips. It is empty on buses and is hidden on short screens.
- **Pan knob:** absent on Input and Master, which have more than 2 channels (Sushi has no pan there). A blank placeholder of the same height keeps the strips aligned.
- **Fader with meter:** the meter shows L/R, a peak-hold tick and a clip LED. Tap the LED to clear it.
- **dB readout and M / S.**

**Groups**, separated by thin vertical labels:
- **Input (advanced only):** Sushi's pre track, shown first with a darker frame.
- **TRACKS:** audio and MIDI tracks, followed by the "＋ Add track or bus" tile.
- **BUSES:** new buses are always added at the end.
- **Master:** pinned on the right. By default it is only a fader and mute, because the post-track chain is hidden. In advanced mode it shows its chain chips and its name opens Track mode for its effects.

**Solo:**
- Solo is additive: tap S on several tracks.
- When anything is soloed, every non-soloed audio or MIDI track is silenced.
- Buses are solo-safe, so a soloed guitar still has its reverb.
- Input and Master have no solo.

### Phone portrait
```
┌──────────────────────────────┐
│ Live Rig*          ▶ 23%  ●  │
├──────────────────────────────┤
│▮ Guitar  AUDIO · IN 1  [M][S]│
│  (◠)  ▬▬▬▬▬▬▬▬▬░░░░  -3.5 dB │  ← pan knob + horizontal fader with meter inside
│  L20                         │
│▮ Vocal   AUDIO · IN 2  [M][S]│
│  (◠)  ▬▬▬▬▬▬▬▬▬▬▬░░   0.0 dB │
│   ＋ Add track or bus         │
│ BUSES                        │
│▮ Reverb  BUS · 4 SENDS [M][S]│
│  (◠)  ▬▬▬▬▬▬▬▬░░░░░  -2.0 dB │
├──────────────────────────────┤
│ MASTER ▬▬▬▬▬▬▬▬▬▬░ -0.5 dB [M]│  ← docked; “MASTER ›” opens it in advanced mode
└──────────────────────────────┘
```
- **Opening a track:** tap the row anywhere outside its controls.
- **Fader:** the gain fader is a horizontal relative drag, and the meter is drawn behind its thumb.
- **Pan knob:** a 32 px knob with its value printed below. A tap opens the Focus slider (§7).
- **Send levels** are not shown in phone rows. Edit them in Track mode under Sends.
- **Quick actions:** swiping a row left reveals Delete.

### Add sheet (track or bus)
1. Pick a kind: **Audio** (input → effects), **MIDI** (keyboard → instrument → effects) or **Bus** (shared effects fed by sends).
2. **Audio:**
   - name (auto "Audio 3");
   - mono/stereo;
   - input channels (from `GetInputAudioChannelCount`);
   - output pair.
3. **MIDI:**
   - name;
   - MIDI port and channel (Omni by default);
   - instrument, chosen from the instrument-role plugins, or "None for now";
   - output pair.
4. **Bus:** name (auto "Bus 2"), first effect (reverb, delay, chorus or none) and output pair. Stereo is assumed.
5. **Create:**
   - The strip appears with a spinner until Sushi confirms it.
   - On a phone, a new track opens in Track mode.
   - On desktop the view stays in the mixer.
   - If the bus was created from a track's "New bus…" action, the app returns to that track with a send already added.

### Empty project
A centred call to action, "Start with a track", with Audio and MIDI as two large cards. Buses aren't offered here because they need tracks to feed them. Master is already visible.
Under the cards, when saved projects exist, a quiet "…or resume a saved project" link expands the list of projects in place; tapping one loads it. The project menu in the top bar is available too.
The project starts empty because the backend deletes the placeholder track that Sushi's JSON config needs (its schema rejects an empty track list); users never see it.

## 5. Track mode

Track mode is for the plugin chain and sends only. It shows the chain at full width, with no input block and no channel strip by default.

### Desktop / tablet / phone landscape
```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ Live Rig* / Mixer › Synth                               ♩ 120.0  ▶ ■   CPU 23%   ●   │
├──────────────────────────────────────────────────────────────────────────────────────┤
│ [‹] ■ Synth [›]  MIDI · PORT 0 · OMNI                    ▬▬▬▬▬░░  [Mix & routing]    │  ← track header
├──────────────────────────────────────────────────────────────────────────────────────┤
│ MIDI EFFECTS       ┊ INSTRUMENT          ┊ AUDIO EFFECTS     ┊ SENDS                  │
│ ┌Arpeggiator─┐ ＋  ┊ ┌Simple Synth──┐    ┊ ┌Chorus──┐  ＋    ┊ ┌→ Reverb─┐┌→ Delay─┐ ＋│
│ │⏻        ⋮⋮ │     ┊ │⏻ ‹Warm Pad› ⋮⋮│    ┊ │⏻     ⋮⋮│       ┊ │ (◠) Pre ││(◠) Pre │  │
│ │(◠)(◠)      │     ┊ │(◠)(◠)(◠)     │    ┊ │(◠)(◠)  │       ┊ │ Level Post││Level Post│ │
│ └────────────┘     ┊ └──────────────┘    ┊ └────────┘       ┊ └─────✕──┘└────✕───┘  │
└──────────────────────────────────────────────────────────────────────────────────────┘
```

**Track header**, always visible:
- ‹ previous track and › next track buttons. On desktop the `[` and `]` keys do the same; on touch, swipe the chain area horizontally (more than 70 px, mostly horizontal).
- The order is the mixer order: [Input] → tracks → buses → [Master], with Input and Master included only in advanced mode.
- The track colour, name and kind badge.
- A compact horizontal meter, so levels stay visible.
- A **Mix & routing** toggle, off by default. The setting is remembered per browser.

**Mix & routing (when on):**
- An input block on the left (AUDIO IN / MIDI IN / FROM SENDS / ENGINE IN) and an OUT block on the right of the chain.
- A right-hand panel (desktop/tablet) with:
  - the fader, meter, pan and M/S;
  - colour;
  - routing (input, output, channels);
  - a "Hidden" line listing the hidden processors (level meter, n sends, return), for transparency;
  - delete track or bus.
- On phone, the same controls appear in a dock at the bottom: pan, fader with meter, dB, M/S.

**Chain sections:**
- **MIDI track:** MIDI effects → Instrument (at most one) → Audio effects → Sends.
- **Audio track:** Audio effects → Sends.
- **Bus, Input, Master:** Audio effects only. A bus can't send to another bus in v1.

**Plugin card:**
- Bypass toggle; a bypassed card is dimmed.
- Name; tap it to open Plugin mode.
- Program selector with ‹ › and a list, shown when `program_count > 0`.
- 2–6 pinned controls, chosen in Plugin mode.
- A trash button. It asks "Remove X? Remove / Cancel" inline on the card; there is no modal. Removing the instrument brings back its ＋ slot.
- A ⋮⋮ drag handle. Cards can only be dropped within their own section, so a generator can't end up after an effect. The handle is disabled when the section has one card. Alt + arrow keys on the focused handle move the card one slot.

**＋ slot:**
- One at the end of each section. It opens the plugin browser filtered by that section's role.
- The Instrument section accepts one plugin only; replacing it asks for confirmation.

**Send card:**
- "→ Bus name" with the bus's effects as a subtitle. Tapping it opens that bus in Track mode.
- Level knob in dB.
- **Pre / Post** fader selector. Post is the default.
- ✕ removes the send.

**Sends ＋ slot:**
- Opens the Add-send sheet, which lists buses that don't have a send from this track yet.
- The last row, **New bus…**, opens the Add sheet in Bus mode and then adds the send.
- New sends start at −12 dB, post-fader.

### Phone portrait
```
┌──────────────────────────────┐
│ ‹ Mixer              ▶ 23% ● │
├──────────────────────────────┤
│ [‹] ■ Synth [›]  ▬▬░  [Mix]  │
├──────────────────────────────┤
│ MIDI EFFECTS                 │
│ ┌⏻ Arpeggiator        ›  ⋮⋮┐ │
│ │  (◠) (◠)                 │ │
│ └──────────────────────────┘ │
│   ＋ Add MIDI effect          │
│ INSTRUMENT                   │
│ ┌⏻ Simple Synth  ‹Pad› › ⋮⋮┐ │
│ │  (◠)cut (◠)res (◠)atk    │ │
│ └──────────────────────────┘ │
│ AUDIO EFFECTS                │
│ ┌⏻ Chorus             ›  ⋮⋮┐ │  collapsed cards show the header only
│   ＋ Add effect               │
│ SENDS                        │
│ ┌→ Reverb            ✕     ┐ │
│ │  (◠) Level    [Pre][Post]│ │
│   ＋ Add send                 │
└──────────────────────────────┘
   (dock with pan · fader · M S appears only when [Mix] is on)
```
- **Collapsing:** tapping a card's header collapses or expands it. The first card of each section starts expanded.
- **Reordering:** long-press the ⋮⋮ handle (about a quarter of a second) and drag vertically. An accent line shows where the card will land, and the pane scrolls near its edges.
- **Changing track:** swipe horizontally on the chain, or use the ‹ › buttons.

## 6. Plugin mode

### Desktop
```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Live Rig* › Mixer › Synth › Simple Synth            ♩120 ▶ ■  CPU 23% ● live │
├───────────────┬──────────────────────────────────────────────────────────────┤
│ PROGRAMS      │ ⏻ [‹] Simple Synth [›]              [filter params…]    ║▌▌  │
│  Init         │ ─ Voice ──────────────────────────────────────────────────── │
│ ▸ Warm Pad    │  (◠)     (◠)      (◠)                                         │
│  Bright Lead  │  Volume  Glide    Pulse width                                 │
│  Pluck        │ ─ Filter ─────────────────────────────────────────────────── │
│  Sub Bass     │  (◠)     (◠)                                                  │
│               │  Cutoff  Resonance                                            │
│ [Choose card  │ ─ Envelope ───────────────────────────────────────────────── │
│  controls]    │  (◠)     (◠)      (◠)      (◠)                                │
│               │  Attack  Decay    Sustain  Release                            │
└───────────────┴──────────────────────────────────────────────────────────────┘
```
**Header:**
- Bypass.
- ‹ › to move to the previous or next plugin in the same track.
- Plugin name, plus vendor and format on desktop.
- Parameter filter.
- The track's meter.

**Parameter grid:**
- FLOAT parameters are knobs, BOOL are toggles, and INT are steppers.
- An INT whose `formatted_value` strings look like named choices (e.g. waveforms) becomes a dropdown.
- Properties (strings) get a text field and a file picker.
- Each control shows its label and Sushi's `formatted_value`, e.g. "1.2 kHz".

**Groups:**
- Parameters are grouped by a shared name prefix (`osc_`, `filter_`, `env_`).
- If there's no prefix, they are split into chunks of 8 in the plugin's own order.

**Other rules:**
- **Outputs:** non-automatable (output) parameters appear in a separate "Outputs" group as read-only values or mini meters.
- **Large plugins** (100+ parameters): the grid is virtualized, searchable, and groups start collapsed.
- **"Choose card controls":** a selection mode where you tap up to 6 controls to pin them to the Track-mode card.
- **Remove:** a Remove button in the header, with the same inline confirmation as on the card. After removal the view returns to the track.
- **Desktop extras:** mouse wheel on knobs, Shift for fine control, double-click to reset.

### Phone portrait
```
┌──────────────────────────────┐
│ ‹ Synth              ▶ 23% ● │
├──────────────────────────────┤
│ ⏻ [‹] Simple Synth [›]  ║▌  │
│ [‹ Warm Pad ›] [Choose…]     │
├──────────────────────────────┤
│ VOICE                        │
│   ( ◠ )        ( ◠ )         │
│   Volume -8dB  Glide 0 ms    │
│ FILTER                       │
│   ( ◠ )        ( ◠ )         │
│   Cutoff 1.2k  Reso 1.40     │
└──────────────────────────────┘
       tap a knob ⇒
┌──────────────────────────────┐
│  Cutoff                  ✕   │
│         1.20 kHz             │
│ ├────────────●──────────────┤│  ← Focus slider: full width, relative drag
│  Shift / 2nd finger = fine   │
│  [ Reset ]          [ Done ] │
└──────────────────────────────┘
```

## 7. Widgets — interaction spec

| Widget | Size (min) | Gesture | Feedback | Reset |
|--------|-----------|---------|----------|-------|
| Knob | 44 px (desktop 40, phone 64 in plugin grid, 32 in phone mixer row) | vertical drag, 200 px = full range; Shift / 2nd finger = ×0.1 fine; wheel; arrow keys | arc fill, value text | double-tap → default |
| Mini send knob | 26 px (desktop strip only) | as knob | arc only; value while dragging | double-tap → −12 dB |
| Pan knob | 32–44 px | as knob, centre detent ±2 % | "L 30 / C / R 30" | double-tap → C |
| Fader | 26×20 px thumb | relative drag; no jump on tap | dB readout; unity snap ±0.25 dB | double-tap → 0 dB |
| Horizontal fader (phone rows, docks, Focus) | full width × 40 px | relative drag; captured after 6 px horizontal movement (vertical scrolls the list) | meter drawn behind thumb | double-tap → 0 dB / default |
| Toggle (M/S/bypass/Pre-Post) | 40×30 | tap | M amber, S blue, Pre/Post amber outline, bypass dims card | — |
| ‹ › navigation | 36×36 | tap; swipe on phone; `[` `]` keys | disabled at the ends | — |
| Program selector | — | ‹ › or tap name → searchable list | name updates when Sushi confirms | — |
| Meter | 14 px wide per strip pair; 12 px tall in headers | tap clip LED to clear | green → amber (−12 dB) → red (−3 dB); peak tick holds 1.5 s; clip LED latches | — |

**Fader law** maps fader position to dB in four segments:

| dB range | Share of fader travel |
|----------|-----------------------|
| +24 to 0 dB | top 25 % |
| 0 to −20 dB | next 45 % |
| −20 to −60 dB | next 25 % |
| −60 to −120 dB | last 5 % |

The UI converts the fader position to dB, then to Sushi's normalized value. The same law is used for send levels.

**Meter scale:** −60 to +6 dB, with ticks at +12, +6, 0, −6, −12, −20, −40 and −60 next to the fader.

## 8. States and feedback
- **Pending operation:** e.g. adding a plugin shows a ghost card with a spinner in the target slot. If `result.ok=false`, the ghost becomes an inline error ("Sushi couldn't load 'Dexed.vst3': NOT_FOUND") with Retry and Dismiss.
- **Reconnecting:** amber banner, all controls disabled at 50 % opacity, meters drop to −∞.
- **Engine missing:** a full-screen message, "Sushi isn't running on this device", showing the address being tried.
- **Destructive confirmation:** the button itself turns into a confirmation ("Delete Guitar and its 3 plugins? Delete / Cancel"), with no modal dialogs.
- **Deleting a bus** also removes every send pointing to it. The confirmation names how many sends are affected.
- **Clip:** the clip LED latches on the strip or row; tap to clear. A master clip also shows as a red dot next to the project name, so it's visible from any depth.
- **Unsaved changes:** a dot after the project name; the Project menu offers Save.

## 9. Visual language
- **Theme:** dark-first and low-glare for stage use. A light theme may come later.
- **Colour:**
  - neutral graphite surfaces with a slight blue tint;
  - one warm accent (amber) for interaction, focus and selection;
  - meter colours (green/amber/red) kept separate from the accent.
- **Track colours:** 8 colours, assigned in turn and changeable by the user (buses start from the end of the palette). They are used for the strip tab, the track header swatch, the phone row stripe, and the top border of send cards (in the bus's colour).
- **Bus names** are set in italics in the mixer so buses read differently at a glance.
- **Type:** a condensed sans for labels, so they fit narrow strips, and monospaced tabular figures for values.

## 10. Open questions
1. **Send position:** sends sit at the end of the chain (post-FX). Is a "pre-FX" send position needed for any use case?
2. **Bus → bus sends** (e.g. delay into reverb): v1 says no. Revisit if users ask.
3. ~~Phone bottom tab bar~~: removed (2026-10-15). The Project menu opens from the project name.
4. **Mix & routing memory:** should the toggle be remembered per browser (current proposal) or per project?
5. **Master chain in non-advanced mode:** hidden entirely (current), or show its plugins read-only?

## 11. Deferred to later versions
Snapshots (quick recall of a set of parameter values), MIDI learn (`ConnectCCToParameter`), program change from a MIDI footswitch (`ConnectPCToProcessor`), undo/redo, light theme.

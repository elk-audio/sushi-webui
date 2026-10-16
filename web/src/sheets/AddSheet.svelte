<script lang="ts">
  import { untrack } from 'svelte';
  import type { Sheet } from '../lib/ui.svelte';
  import { ui, go } from '../lib/ui.svelte';
  import { store } from '../lib/state.svelte';
  import { cmd } from '../lib/net/socket';
  import { ICON } from '../lib/icons';
  import { chanLabel } from '../lib/util';
  import SheetFrame from './Sheet.svelte';

  let { sheet }: { sheet: Extract<Sheet, { kind: 'add' }> } = $props();
  let s = $derived(store.state!);

  const defName = (k: string) =>
    ({ midi: 'MIDI ', audio: 'Audio ', bus: 'Bus ' } as Record<string, string>)[k] + (store.state!.tracks.filter(t => t.kind === k).length + 1);

  const initialKind = untrack(() => sheet.trackKind);  // the sheet opens with this kind; the user can switch
  let kind = $state<'audio' | 'midi' | 'bus'>(initialKind);
  let name = $state(defName(initialKind));
  let nameEdited = $state(false);
  let channels = $state<1 | 2>(1);
  let input = $state(store.state!.sushi.defaultInput);
  let output = $state(store.state!.sushi.defaultOutput);
  let midiPort = $state(0);
  let midiChannel = $state(0);
  const cat = store.state!.catalog;
  let instrument = $state(initialKind === 'midi' ? (cat.find(c => c.role === 'instrument')?.id ?? '') : '');
  let fx = $state(cat.find(c => c.role === 'audio_fx' && /reverb/i.test(c.name))?.id ?? '');
  let busy = $state(false);

  let instruments = $derived(s.catalog.filter(c => c.role === 'instrument'));
  let effects = $derived(s.catalog.filter(c => c.role === 'audio_fx'));
  let inOpts = $derived.by(() => {
    const n = Math.max(1, s.sushi.inputs);
    const step = channels;
    const o: { v: number; l: string }[] = [];
    for (let i = 0; i + step <= n; i += step) o.push({ v: i, l: chanLabel('In', i, step) });
    return o;
  });
  let outOpts = $derived.by(() => {
    const n = Math.max(2, s.sushi.outputs);
    const o: { v: number; l: string }[] = [];
    for (let i = 0; i + 2 <= n; i += 2) o.push({ v: i, l: chanLabel('Out', i, 2) });
    return o;
  });
  let sendFrom = $derived(sheet.sendFrom !== undefined ? s.tracks.find(t => t.id === sheet.sendFrom) : undefined);

  function pickKind(k: 'audio' | 'midi' | 'bus') {
    kind = k;
    if (!nameEdited) name = defName(k);
    if (k === 'midi' && !instrument && instruments.length) instrument = instruments[0].id;
  }
  $effect(() => { if (input + channels > Math.max(1, s.sushi.inputs)) input = 0; });
  $effect(() => { if (output + 2 > Math.max(2, s.sushi.outputs)) output = 0; });

  async function create() {
    busy = true;
    const args: Record<string, unknown> = { kind, name: name.trim() || defName(kind), output };
    if (kind === 'audio') { args.channels = channels; args.input = input; }
    if (kind === 'midi') { args.midiPort = midiPort; args.midiChannel = midiChannel; if (instrument) args.instrument = instrument; }
    if (kind === 'bus' && fx) args.fx = fx;
    if (sheet.sendFrom !== undefined) args.sendFrom = sheet.sendFrom;
    try {
      const r = (await cmd('addTrack', args)) as { id?: number } | undefined;
      ui.sheet = null;
      // UX_DESIGN §4: on phone a new track opens in Track mode (not when created from "New bus…")
      if (ui.size === 'phone' && sheet.sendFrom === undefined && r?.id !== undefined) go({ view: 'track', t: String(r.id) });
    } catch { /* toasted */ } finally { busy = false; }
  }
</script>

<SheetFrame title={sendFrom ? `New bus for ${sendFrom.name}` : 'Add track'}>
  <div class="kindcards three">
    <button class="kindcard" aria-pressed={kind === 'audio'} onclick={() => pickKind('audio')}>{@html ICON.audio}<b>Audio</b><span>Input → effects</span></button>
    <button class="kindcard" aria-pressed={kind === 'midi'} onclick={() => pickKind('midi')}>{@html ICON.midi}<b>MIDI</b><span>Keyboard → instrument → effects</span></button>
    <button class="kindcard" aria-pressed={kind === 'bus'} onclick={() => pickKind('bus')}>{@html ICON.bus}<b>Bus</b><span>Shared effects fed by sends</span></button>
  </div>
  <label class="field"><span>Name</span><input id="add-name" bind:value={name} oninput={() => (nameEdited = true)} /></label>
  {#if kind === 'audio'}
    <div class="field"><span>Channels</span>
      <div class="opts">
        <button aria-pressed={channels === 1} onclick={() => { channels = 1; }}>Mono</button>
        <button aria-pressed={channels === 2} onclick={() => { channels = 2; input = input - (input % 2); }}>Stereo</button>
      </div>
    </div>
    <div class="fieldrow">
      <label class="field"><span>Input</span><select id="add-in" bind:value={input}>{#each inOpts as o}<option value={o.v}>{o.l}</option>{/each}</select></label>
      <label class="field"><span>Output</span><select id="add-out" bind:value={output}>{#each outOpts as o}<option value={o.v}>{o.l}</option>{/each}</select></label>
    </div>
  {:else if kind === 'midi'}
    <div class="fieldrow">
      <label class="field"><span>MIDI input</span><select id="add-port" bind:value={midiPort}>
        {#each Array.from({ length: Math.max(1, s.sushi.midiInputs) }, (_, i) => i) as p}<option value={p}>Port {p}</option>{/each}
      </select></label>
      <label class="field"><span>Channel</span><select id="add-mch" bind:value={midiChannel}>
        <option value={0}>Omni</option>{#each Array.from({ length: 16 }, (_, i) => i + 1) as c}<option value={c}>{c}</option>{/each}
      </select></label>
    </div>
    <div class="field"><span>Instrument</span>
      <div class="opts">
        {#each instruments as c (c.id)}<button aria-pressed={instrument === c.id} onclick={() => (instrument = c.id)}>{c.name}</button>{/each}
        <button aria-pressed={instrument === ''} onclick={() => (instrument = '')}>None for now</button>
      </div>
    </div>
    <label class="field"><span>Output</span><select id="add-out-m" bind:value={output}>{#each outOpts as o}<option value={o.v}>{o.l}</option>{/each}</select></label>
  {:else}
    <div class="field"><span>First effect</span>
      <div class="opts">
        {#each effects.slice(0, 12) as c (c.id)}<button aria-pressed={fx === c.id} onclick={() => (fx = c.id)}>{c.name}</button>{/each}
        <button aria-pressed={fx === ''} onclick={() => (fx = '')}>None for now</button>
      </div>
    </div>
    <label class="field"><span>Output</span><select id="add-out-b" bind:value={output}>{#each outOpts as o}<option value={o.v}>{o.l}</option>{/each}</select></label>
    <p class="note">A bus has no audio input of its own. Tracks feed it from the Sends section in Track view.</p>
  {/if}
  {#if kind !== 'bus'}<p class="note">The track gets a hidden level meter at the end of its chain. Sushi reports {s.sushi.inputs} inputs and {s.sushi.outputs} outputs.</p>{/if}
  {#snippet footer()}
    <button class="secondary" onclick={() => (ui.sheet = null)}>Cancel</button>
    <button class="primary" disabled={busy} onclick={create}>{busy ? 'Creating…' : `Create ${kind === 'midi' ? 'MIDI track' : kind === 'bus' ? 'bus' : 'audio track'}`}</button>
  {/snippet}
</SheetFrame>

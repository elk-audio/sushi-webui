<script lang="ts">
  import { untrack } from 'svelte';
  import { paramText } from '../lib/util';
  import type { Param, Processor, Track } from '../lib/types';
  import { store, toast } from '../lib/state.svelte';
  import { ui, go, trackKey } from '../lib/ui.svelte';
  import { act, cmd } from '../lib/net/socket';
  import { confirmKey, removePlugin } from '../lib/actions';
  import { paramCtl } from '../lib/ctl';
  import { ICON } from '../lib/icons';
  import Knob from '../lib/widgets/Knob.svelte';
  import Stepper from '../lib/widgets/Stepper.svelte';
  import Meter from '../lib/widgets/Meter.svelte';

  let { track, proc }: { track: Track; proc: Processor } = $props();
  let s = $derived(store.state!);
  let phone = $derived(ui.size === 'phone');
  let tkey = $derived(trackKey(s, track));
  let i = $derived(track.chain.findIndex(p => p.id === proc.id));
  let vendor = $derived(s.catalog.find(c => c.uid === proc.uid)?.vendor || (proc.type === 'internal' ? 'Sushi internal' : ''));

  // Groups: shared name prefix before '_' when at least two params share it, else chunks of 8.
  let groups = $derived.by(() => {
    const q = ui.filter.trim().toLowerCase();
    const ins = proc.params.filter(p => p.automatable && (!q || (p.label || p.name).toLowerCase().includes(q)));
    const outs = proc.params.filter(p => !p.automatable && (!q || (p.label || p.name).toLowerCase().includes(q)));
    const prefix = (p: Param) => (p.name.includes('_') ? p.name.split('_')[0] : '');
    const counts = new Map<string, number>();
    for (const p of ins) counts.set(prefix(p), (counts.get(prefix(p)) ?? 0) + 1);
    const usePrefix = [...counts.entries()].some(([k, n]) => k && n >= 2);
    const out: { name: string; params: Param[] }[] = [];
    if (usePrefix) {
      const m = new Map<string, Param[]>();
      for (const p of ins) {
        const k = (counts.get(prefix(p)) ?? 0) >= 2 ? prefix(p) : '';
        if (!m.has(k)) m.set(k, []);
        m.get(k)!.push(p);
      }
      for (const [k, ps] of m) out.push({ name: k ? k.replace(/^\w/, c => c.toUpperCase()) : 'Parameters', params: ps });
    } else {
      for (let j = 0; j < ins.length; j += 8)
        out.push({ name: ins.length > 8 ? `Parameters ${j + 1}–${Math.min(j + 8, ins.length)}` : 'Parameters', params: ins.slice(j, j + 8) });
    }
    if (outs.length) out.push({ name: 'Outputs', params: outs });
    return out;
  });

  function setBypass() { proc.bypass = !proc.bypass; act('setBypass', { proc: proc.id, on: proc.bypass }); }
  function setProgram(n: number) { proc.program = n; act('setProgram', { proc: proc.id, program: n }); }
  function stepProgram(d: number) {
    const n = proc.programs.length;
    if (n) setProgram((proc.program + d + n) % n);
  }
  function togglePin(id: number) {
    const has = proc.macros.includes(id);
    if (!has && proc.macros.length >= 6) { toast('A card shows at most 6 controls'); return; }
    proc.macros = has ? proc.macros.filter(x => x !== id) : [...proc.macros, id];
    act('setMacros', { proc: proc.id, params: proc.macros });
  }
  function setBool(p: Param) {
    p.n = p.n >= 0.5 ? 0 : 1;
    paramCtl(proc.id, p).set(p.n);
  }
  // Wav Streamer: the file property is a picker over the backend's audio folder; choosing a file also
  // starts looped playback (the backend sets "playing" and "loop").
  let isWav = $derived(proc.uid === 'sushi.testing.wav_streamer');
  let wavs = $state<string[] | null>(null);
  let wavBusy = $state(false);
  async function loadWavs() {
    try { wavs = ((await cmd('listWavs')) as { files: string[] }).files; } catch { wavs = []; }
  }
  // untrack: cmd() reads and writes store.busy, which would otherwise make this effect re-run forever
  $effect(() => { if (isWav) untrack(loadWavs); });
  async function pickWav(file: string) {
    if (!file) return;
    wavBusy = true;
    try { await cmd('setWavFile', { proc: proc.id, value: file }); } catch { /* toasted */ } finally { wavBusy = false; }
  }
  // The property holds an absolute path; show it relative to the picker's list.
  const wavValue = (v: string) => (wavs ?? []).find(f => v === f || v.endsWith('/' + f)) ?? '';
  function commitProp(id: number, value: string) {
    cmd('setProperty', { proc: proc.id, prop: id, value }).catch(() => {});
  }
</script>

<div class="pluginview">
  {#if !phone && proc.programs.length}
    <aside class="progs">
      <h4>Programs</h4>
      {#each proc.programs as name, n}
        <button aria-current={n === proc.program} onclick={() => setProgram(n)}>{name}</button>
      {/each}
      <button class="pinbtn ghost-btn" aria-pressed={ui.pinMode} onclick={() => (ui.pinMode = !ui.pinMode)}>{ui.pinMode ? 'Done choosing' : 'Choose card controls'}</button>
    </aside>
  {/if}
  <div class="pmain">
    <div class="phead">
      <button class="pwr" aria-pressed={!proc.bypass} aria-label="Bypass" style={proc.bypass ? 'color:var(--faint)' : ''} onclick={setBypass}>{@html ICON.power}</button>
      <button class="nav" disabled={i <= 0} aria-label="Previous plugin" onclick={() => go({ view: 'plugin', t: tkey, p: track.chain[i - 1].id })}>‹</button>
      <span class="pn">{proc.name}{#if !phone}<small>{vendor}{vendor ? ' · ' : ''}{proc.type.toUpperCase()}</small>{/if}</span>
      <button class="nav" disabled={i >= track.chain.length - 1} aria-label="Next plugin" onclick={() => go({ view: 'plugin', t: tkey, p: track.chain[i + 1].id })}>›</button>
      {#if phone && proc.programs.length}
        <div class="progstep">
          <button class="arr" aria-label="Previous program" onclick={() => stepProgram(-1)}>‹</button>
          <button class="pname" onclick={() => (ui.sheet = { kind: 'programs', proc: proc.id })}>{proc.programs[proc.program] ?? '—'}</button>
          <button class="arr" aria-label="Next program" onclick={() => stepProgram(1)}>›</button>
        </div>
      {/if}
      <div class="spacer"></div>
      {#if !phone}<input type="search" placeholder="Filter parameters" aria-label="Filter parameters" bind:value={ui.filter} />{/if}
      {#if phone || !proc.programs.length}
        <button class="pinbtn ghost-btn" aria-pressed={ui.pinMode} onclick={() => (ui.pinMode = !ui.pinMode)}>{ui.pinMode ? 'Done' : 'Choose card controls'}</button>
      {/if}
      {#if ui.confirm === confirmKey(proc.id)}
        <div class="confirm">Remove {proc.name}?
          <button class="yes" onclick={() => removePlugin(proc.id)}>Remove</button>
          <button class="no" onclick={() => (ui.confirm = '')}>Cancel</button></div>
      {:else}
        <button class="rm" aria-label="Remove {proc.name}" title="Remove plugin" onclick={() => (ui.confirm = confirmKey(proc.id))}>{@html ICON.trash}{#if !phone}<span>Remove</span>{/if}</button>
      {/if}
      {#if track.meter >= 0}<Meter slot={track.meter} orient="v" title="{track.name} level" />{/if}
      {#if ui.pinMode}<div class="pinhint">Tap up to 6 controls to show on the {track.name} track card. {proc.macros.length}/6 chosen.</div>{/if}
    </div>
    <div class="groups">
      {#each groups as g (g.name)}
        <section class="group">
          <h5>{g.name}</h5>
          <div class="pgrid">
            {#each g.params as p (p.id)}
              {#if !p.automatable}
                <div class="pctl"><span class="outval">{paramText(p)}</span><span class="lab">{p.label || p.name}</span></div>
              {:else if ui.pinMode}
                <Knob ctl={paramCtl(proc.id, p)} size={phone ? 64 : 52} pickable pinned={proc.macros.includes(p.id)} onpick={() => togglePin(p.id)} />
              {:else if p.type === 'bool'}
                <div class="pctl"><button class="bigtog" class:on={p.n >= 0.5} aria-pressed={p.n >= 0.5} onclick={() => setBool(p)}>{p.n >= 0.5 ? 'On' : 'Off'}</button><span class="lab">{p.label || p.name}</span></div>
              {:else if p.type === 'int'}
                <Stepper ctl={paramCtl(proc.id, p)} />
              {:else}
                {@const c = paramCtl(proc.id, p)}
                <Knob ctl={c} size={phone ? 64 : 52} pinned={proc.macros.includes(p.id)}
                  onTap={phone ? () => (ui.sheet = { kind: 'focus', ctl: c }) : undefined} />
              {/if}
            {/each}
          </div>
        </section>
      {:else}
        <p class="note">{ui.filter ? `No parameters match “${ui.filter}”.` : 'This plugin has no parameters.'}</p>
      {/each}
      {#if proc.props.length && !ui.filter}
        <section class="group">
          <h5>Properties</h5>
          <div class="pgrid">
            {#each proc.props as pr (pr.id)}
              <div class="prop">
                <label for="prop-{proc.id}-{pr.id}">{pr.label || pr.name}</label>
                {#if isWav && pr.name === 'file'}
                  <select id="prop-{proc.id}-{pr.id}" disabled={wavBusy || !wavs?.length} value={wavValue(pr.value)} onchange={e => pickWav(e.currentTarget.value)}>
                    <option value="" disabled>{wavs === null ? 'Loading…' : wavs.length ? 'Choose a wav file' : 'No wav files in the audio folder'}</option>
                    {#each wavs ?? [] as f (f)}<option value={f}>{f}</option>{/each}
                  </select>
                  <button class="ghost-btn" disabled={wavBusy} onclick={loadWavs}>Rescan</button>
                {:else}
                <input id="prop-{proc.id}-{pr.id}" value={pr.value}
                  onkeydown={e => { if (e.key === 'Enter') (e.currentTarget as HTMLInputElement).blur(); }}
                  onchange={e => commitProp(pr.id, (e.currentTarget as HTMLInputElement).value)} />
                {/if}
              </div>
            {/each}
          </div>
        </section>
      {/if}
    </div>
  </div>
</div>

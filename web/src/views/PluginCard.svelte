<script lang="ts">
  import type { Processor } from '../lib/types';
  import { ui, go } from '../lib/ui.svelte';
  import { act } from '../lib/net/socket';
  import { paramCtl } from '../lib/ctl';
  import { ICON } from '../lib/icons';
  import Knob from '../lib/widgets/Knob.svelte';

  let { proc, tkey, collapsed = false, ontoggle, onhandle }: {
    proc: Processor; tkey: string; collapsed?: boolean; ontoggle?: () => void;
    onhandle?: (e: PointerEvent, el: HTMLElement) => void;
  } = $props();
  let phone = $derived(ui.size === 'phone');
  let card: HTMLElement;
  let macros = $derived(proc.macros.map(id => proc.params.find(p => p.id === id)).filter(p => p !== undefined));
  const sub = $derived(proc.type === 'internal' ? '' : proc.type.toUpperCase());

  function setBypass() { proc.bypass = !proc.bypass; act('setBypass', { proc: proc.id, on: proc.bypass }); }
  function step(d: number) {
    const n = proc.programs.length;
    if (!n) return;
    proc.program = (proc.program + d + n) % n;
    act('setProgram', { proc: proc.id, program: proc.program });
  }
</script>

<div class="card" class:byp={proc.bypass} class:collapsed={phone && collapsed} bind:this={card} data-proc={proc.id}>
  <div class="card-head">
    <button class="pwr" aria-pressed={!proc.bypass} aria-label="Bypass {proc.name}" onclick={setBypass}>{@html ICON.power}</button>
    {#if phone}
      <button class="card-name" onclick={() => ontoggle?.()}>{proc.name}{#if sub}<small>{sub}</small>{/if}</button>
      <button class="pwr" style="color:var(--text)" aria-label="Open {proc.name}" onclick={() => go({ view: 'plugin', t: tkey, p: proc.id })}>›</button>
    {:else}
      <button class="card-name" onclick={() => go({ view: 'plugin', t: tkey, p: proc.id })}>{proc.name}{#if sub}<small>{sub}</small>{/if}</button>
    {/if}
    <span class="handle" role="button" tabindex="-1" aria-label="Drag to reorder" onpointerdown={e => onhandle?.(e, card)}>{@html ICON.grip}</span>
  </div>
  {#if proc.programs.length}
    <div class="card-prog">
      <button class="arr" aria-label="Previous program" onclick={() => step(-1)}>‹</button>
      <button class="pname" onclick={() => (ui.sheet = { kind: 'programs', proc: proc.id })}>{proc.programs[proc.program] ?? '—'}</button>
      <button class="arr" aria-label="Next program" onclick={() => step(1)}>›</button>
    </div>
  {/if}
  <div class="macros">
    {#each macros as p (p.id)}
      <Knob ctl={paramCtl(proc.id, p)} size={phone ? 40 : 38} />
    {:else}
      <div class="none">No pinned controls</div>
    {/each}
  </div>
</div>

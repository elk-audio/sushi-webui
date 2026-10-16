<script lang="ts">
  import type { Track } from '../types';
  import { store } from '../state.svelte';
  import { act } from '../net/socket';
  import { chanLabel } from '../util';

  let { track, dir }: { track: Track; dir: 'in' | 'out' } = $props();
  let s = $derived(store.state!);
  let input = $derived(dir === 'in');
  let editable = $derived(input ? track.kind === 'audio' : track.kind === 'audio' || track.kind === 'midi' || track.kind === 'bus');
  let current = $derived(input ? track.inChan : track.outChan);
  let text = $derived(input ? track.input : track.output);
  // Stereo tracks pick a pair; a dual-mono track picks a single input.
  let step = $derived(input && track.inMono ? 1 : 2);
  let opts = $derived.by(() => {
    const n = input ? s.sushi.inputs : s.sushi.outputs;
    const o: { v: number; l: string }[] = [];
    for (let i = 0; i + step <= n; i += step) o.push({ v: i, l: chanLabel(input ? 'In' : 'Out', i, step) });
    if (current >= 0 && !o.some(x => x.v === current)) o.unshift({ v: current, l: text });
    return o;
  });

  function change(e: Event) {
    const v = Number((e.currentTarget as HTMLSelectElement).value);
    act('setRouting', { track: track.id, [input ? 'input' : 'output']: v });
  }
</script>

{#if editable && opts.length > 1}
  <select class="route-sel" aria-label="{track.name} {input ? 'input' : 'output'}" value={current} onchange={change}>
    {#each opts as o (o.v)}<option value={o.v}>{o.l}</option>{/each}
  </select>
{:else}
  <span>{text}</span>
{/if}

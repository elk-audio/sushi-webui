<script lang="ts">
  import type { Ctl } from '../types';
  import { drag } from '../drag';
  import { posFromDb } from '../util';
  import Meter from './Meter.svelte';

  let { ctl, slot }: { ctl: Ctl; slot: number } = $props();
  const ticks = [12, 6, 0, -6, -12, -20, -40, -60];
</script>

<div class="fz">
  <div class="scale" aria-hidden="true">
    {#each ticks as d}<span style="--p:{posFromDb(d)}">{d > 0 ? '+' + d : d}</span>{/each}
  </div>
  <div class="vfader" role="slider" tabindex="0" aria-label={ctl.label} aria-valuetext={ctl.fmt()} aria-valuenow={Math.round(ctl.get() * 100)}
    style="--v:{ctl.get()}" use:drag={{ ctl, mode: 'vfader' }}>
    <div class="thumb"></div>
  </div>
  {#if slot >= 0}<Meter {slot} orient="v" />{/if}
</div>

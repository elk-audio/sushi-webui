<script lang="ts">
  import type { Ctl } from '../types';
  import { drag } from '../drag';
  import Meter from './Meter.svelte';

  let { ctl, slot = -1, zero = undefined, cls = '' }: { ctl: Ctl; slot?: number; zero?: number; cls?: string } = $props();
</script>

<div class="hfader {cls}" role="slider" tabindex="0" aria-label={ctl.label} aria-valuetext={ctl.fmt()} aria-valuenow={Math.round(ctl.get() * 100)}
  style="--v:{ctl.get()}" use:drag={{ ctl, mode: 'hfader' }}>
  {#if slot >= 0}<Meter {slot} orient="h" />{/if}
  {#if zero !== undefined}<div class="zero" style="--z:{zero}"></div>{/if}
  <div class="hthumb"></div>
</div>

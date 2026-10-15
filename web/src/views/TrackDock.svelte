<script lang="ts">
  import type { Track } from '../lib/types';
  import { toggleMute, toggleSolo } from '../lib/actions';
  import { gainCtl, panCtl } from '../lib/ctl';
  import { isFixed } from '../lib/labels';
  import { posFromDb } from '../lib/util';
  import HFader from '../lib/widgets/HFader.svelte';
  import Knob from '../lib/widgets/Knob.svelte';

  let { track }: { track: Track } = $props();
  let g = $derived(gainCtl(track));
  let p = $derived(panCtl(track));
</script>

<div class="dock"><div class="tdock">
  {#if p}<Knob ctl={p} size={32} label="" />{/if}
  {#if g}<HFader ctl={g} slot={track.meter} zero={posFromDb(0)} /><span class="db">{g.fmt()}</span>{/if}
  <button class="tog m" class:on={track.mute} aria-pressed={track.mute} onclick={() => toggleMute(track)}>M</button>
  {#if !isFixed(track)}
    <button class="tog s" class:on={track.solo} aria-pressed={track.solo} onclick={() => toggleSolo(track)}>S</button>
  {/if}
</div></div>

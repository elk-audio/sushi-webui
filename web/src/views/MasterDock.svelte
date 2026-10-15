<script lang="ts">
  import type { Track } from '../lib/types';
  import { store } from '../lib/state.svelte';
  import { go } from '../lib/ui.svelte';
  import { toggleMute } from '../lib/actions';
  import { gainCtl } from '../lib/ctl';
  import { posFromDb } from '../lib/util';
  import HFader from '../lib/widgets/HFader.svelte';

  let { master }: { master: Track } = $props();
  let g = $derived(gainCtl(master));
</script>

<div class="dock"><div class="mdock">
  {#if store.state?.ui.advanced}
    <button class="lbl" style="color:var(--accent)" onclick={() => go({ view: 'track', t: 'master' })}>MASTER ›</button>
  {:else}
    <span class="lbl">MASTER</span>
  {/if}
  {#if g}<HFader ctl={g} slot={master.meter} zero={posFromDb(0)} /><span class="db">{g.fmt()}</span>{/if}
  <button class="tog m" class:on={master.mute} aria-pressed={master.mute} onclick={() => toggleMute(master)}>M</button>
</div></div>

<script lang="ts">
  import type { Track } from '../lib/types';
  import { store } from '../lib/state.svelte';
  import { go, trackKey, ui } from '../lib/ui.svelte';
  import { gainCtl, panCtl } from '../lib/ctl';
  import { isFixed, kindHtml } from '../lib/labels';
  import { toggleMute, toggleSolo, removeTrack, trackConfirmKey, trackConfirmText } from '../lib/actions';
  import { ICON } from '../lib/icons';
  import { posFromDb } from '../lib/util';
  import Knob from '../lib/widgets/Knob.svelte';
  import HFader from '../lib/widgets/HFader.svelte';

  let { track }: { track: Track } = $props();
  let s = $derived(store.state!);
  let g = $derived(gainCtl(track));
  let p = $derived(panCtl(track));

  function open(e: MouseEvent) {
    if ((e.target as HTMLElement).closest('button,[role="slider"]')) return;
    go({ view: 'track', t: trackKey(s, track) });
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<div class="row" style="--tc:{track.color}" role="group" aria-label={track.name} onclick={open}>
  <div class="stripe"></div>
  <div>
    <div class="row-top">
      <button class="nm" onclick={() => go({ view: 'track', t: trackKey(s, track) })}>{track.name}</button>
      <span class="kind">{@html kindHtml(s, track)}</span>
      <div class="spacer"></div>
      <button class="tog m" class:on={track.mute} aria-pressed={track.mute} onclick={() => toggleMute(track)}>M</button>
      {#if !isFixed(track)}<button class="tog s" class:on={track.solo} aria-pressed={track.solo} onclick={() => toggleSolo(track)}>S</button>{/if}
      {#if !isFixed(track)}<button class="rm row-rm" aria-label="Delete {track.name}" title="Delete {track.kind === 'bus' ? 'bus' : 'track'}" onclick={() => (ui.confirm = trackConfirmKey(track))}>{@html ICON.trash}</button>{/if}
    </div>
    {#if ui.confirm === trackConfirmKey(track)}
      <div class="confirm row-confirm" role="alertdialog" aria-label="Delete {track.name}">{trackConfirmText(s, track)}
        <button class="yes" onclick={() => removeTrack(track)}>Delete</button>
        <button class="no" onclick={() => (ui.confirm = '')}>Cancel</button></div>
    {/if}
    <div class="row-bot">
      {#if p}<Knob ctl={p} size={32} label="" />{:else}<div class="panph" style="width:44px"></div>{/if}
      {#if g}<HFader ctl={g} slot={track.meter} zero={posFromDb(0)} /><span class="db">{g.fmt()}</span>{/if}
    </div>
  </div>
</div>

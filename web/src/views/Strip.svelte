<script lang="ts">
  import type { Track } from '../lib/types';
  import { store } from '../lib/state.svelte';
  import { go, trackKey, ui } from '../lib/ui.svelte';
  import { gainCtl, panCtl, sendCtl } from '../lib/ctl';
  import { isFixed, kindHtml } from '../lib/labels';
  import { toggleMute, toggleSolo, removeTrack, trackConfirmKey, trackConfirmText } from '../lib/actions';
  import { ICON } from '../lib/icons';
  import Knob from '../lib/widgets/Knob.svelte';
  import VFader from '../lib/widgets/VFader.svelte';

  let { track }: { track: Track } = $props();
  let s = $derived(store.state!);
  let key = $derived(trackKey(s, track));
  let fixed = $derived(isFixed(track));
  let open = $derived(!fixed || s.ui.advanced);
  let g = $derived(gainCtl(track));
  let p = $derived(panCtl(track));
</script>

<div class="strip {track.kind}" style="--tc:{track.color}">
  <button class="strip-name" disabled={!open} onclick={() => go({ view: 'track', t: key })}>{track.name}</button>
  {#if !fixed}
    <button class="rm strip-rm" aria-label="Delete {track.name}" title="Delete {track.kind === 'bus' ? 'bus' : 'track'}" onclick={() => (ui.confirm = trackConfirmKey(track))}>{@html ICON.trash}</button>
    {#if ui.confirm === trackConfirmKey(track)}
      <div class="strip-confirm confirm" role="alertdialog" aria-label="Delete {track.name}">{trackConfirmText(s, track)}
        <button class="yes" onclick={() => removeTrack(track)}>Delete</button>
        <button class="no" onclick={() => (ui.confirm = '')}>Cancel</button></div>
    {/if}
  {/if}
  <div class="kind">{@html kindHtml(s, track)}</div>
  <div class="chips">
    {#if open}
      {#each track.chain.slice(0, 3) as pr (pr.id)}
        <button class="chip" class:byp={pr.bypass} onclick={() => go({ view: 'plugin', t: key, p: pr.id })}>{pr.name}</button>
      {/each}
      {#if track.chain.length > 3}<button class="chip more" onclick={() => go({ view: 'track', t: key })}>+{track.chain.length - 3} more</button>{/if}
      {#if track.chain.length === 0}<span class="chip more">no plugins</span>{/if}
    {/if}
  </div>
  <div class="sends">
    {#if track.kind === 'audio' || track.kind === 'midi'}
      {#each track.sends.slice(0, 2) as sd (sd.bus)}<Knob ctl={sendCtl(track, sd)} size={26} mini />{/each}
      {#if track.sends.length > 2}<button class="chip more" onclick={() => go({ view: 'track', t: key })}>+{track.sends.length - 2} sends</button>{/if}
    {/if}
  </div>
  {#if p}<Knob ctl={p} size={36} label="" />{:else}<div class="panph" style="height:52px;width:56px"></div>{/if}
  {#if g}<VFader ctl={g} slot={track.meter} />{/if}
  <div class="db">{g ? g.fmt() : ''}</div>
  <div class="ms">
    <button class="tog m" class:on={track.mute} aria-pressed={track.mute} onclick={() => toggleMute(track)}>M</button>
    {#if !fixed}<button class="tog s" class:on={track.solo} aria-pressed={track.solo} onclick={() => toggleSolo(track)}>S</button>{/if}
  </div>
</div>

<script lang="ts">
  import type { Track } from '../lib/types';
  import { store } from '../lib/state.svelte';
  import { ui } from '../lib/ui.svelte';
  import { act } from '../lib/net/socket';
  import { gainCtl, panCtl } from '../lib/ctl';
  import { isFixed, routeIn } from '../lib/labels';
  import { toggleMute, toggleSolo, removeTrack, trackConfirmKey, trackConfirmText } from '../lib/actions';
  import { PALETTE } from '../lib/util';
  import Knob from '../lib/widgets/Knob.svelte';
  import VFader from '../lib/widgets/VFader.svelte';
  import RouteSelect from '../lib/widgets/RouteSelect.svelte';

  let { track }: { track: Track } = $props();
  let s = $derived(store.state!);
  let fixed = $derived(isFixed(track));
  let g = $derived(gainCtl(track));
  let p = $derived(panCtl(track));
  let hidden = $derived(['level meter',
    ...(track.sends.length ? [`${track.sends.length} send${track.sends.length > 1 ? 's' : ''}`] : []),
    ...(track.kind === 'bus' ? ['return'] : [])].join(', '));
  function setColor(c: string) { track.color = c; act('setColor', { track: track.id, color: c }); }
</script>

<aside class="inspector" style="--tc:{track.color}">
  <h4>Mix</h4>
  <div class="insp-mix">
    {#if g}<VFader ctl={g} slot={track.meter} />{/if}
    <div class="insp-side">
      {#if p}<Knob ctl={p} size={40} />{:else}<span></span>{/if}
      <div class="db">{g ? g.fmt() : ''}</div>
      <div class="ms">
        <button class="tog m" class:on={track.mute} aria-pressed={track.mute} onclick={() => toggleMute(track)}>M</button>
        {#if !fixed}<button class="tog s" class:on={track.solo} aria-pressed={track.solo} onclick={() => toggleSolo(track)}>S</button>{/if}
      </div>
    </div>
  </div>
  {#if !fixed}
    <h4>Color</h4>
    <div class="swatches">
      {#each PALETTE as c}<button style="--c:{c}" aria-pressed={c === track.color} aria-label="Color {c}" onclick={() => setColor(c)}></button>{/each}
    </div>
  {/if}
  <h4>Routing</h4>
  <div class="route">
    <span>Input</span>{#if track.kind === 'audio'}<RouteSelect {track} dir="in" />{:else}<b>{routeIn(s, track)[1]}</b>{/if}
    <span>Output</span><RouteSelect {track} dir="out" />
    <span>Channels</span><b>{track.channels === 1 ? 'Mono' : track.channels === 2 ? 'Stereo' : track.channels}</b>
    <span>Hidden</span><b>{hidden}</b>
  </div>
  <div class="spacer"></div>
  {#if !fixed}
    {#if ui.confirm === trackConfirmKey(track)}
      <div class="confirm">{trackConfirmText(s, track)}
        <button class="yes" onclick={() => removeTrack(track)}>Delete</button>
        <button class="no" onclick={() => (ui.confirm = '')}>Cancel</button></div>
    {:else}
      <button class="danger" onclick={() => (ui.confirm = trackConfirmKey(track))}>Delete {track.kind === 'bus' ? 'bus' : 'track'}</button>
    {/if}
  {/if}
</aside>

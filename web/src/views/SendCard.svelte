<script lang="ts">
  import type { Send, Track } from '../lib/types';
  import { store } from '../lib/state.svelte';
  import { go } from '../lib/ui.svelte';
  import { act } from '../lib/net/socket';
  import { sendCtl } from '../lib/ctl';
  import Knob from '../lib/widgets/Knob.svelte';

  let { track, send }: { track: Track; send: Send } = $props();
  let bus = $derived(store.state!.tracks.find(t => t.id === send.bus));
  let ctl = $derived(sendCtl(track, send));
  function mode(post: boolean) { send.post = post; act('setSendMode', { track: track.id, bus: send.bus, post }); }
</script>

<div class="card send" style="--tc:{bus?.color ?? 'var(--line)'}">
  <div class="card-head">
    <button class="card-name" onclick={() => bus && go({ view: 'track', t: String(bus.id) })}>→ {send.busName}
      <small>BUS · {bus?.chain.map(p => p.name).join(', ') || 'no effects'}</small></button>
    <button class="pwr" style="color:var(--muted)" aria-label="Remove send to {send.busName}"
      onclick={() => act('removeSend', { track: track.id, bus: send.bus })}>✕</button>
  </div>
  <div class="sendbody">
    <Knob {ctl} size={40} label="Level" />
    <div class="prepost">
      <button aria-pressed={!send.post} onclick={() => mode(false)}>Pre</button>
      <button aria-pressed={send.post} onclick={() => mode(true)}>Post</button>
    </div>
  </div>
</div>

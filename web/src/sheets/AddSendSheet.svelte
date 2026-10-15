<script lang="ts">
  import type { Sheet } from '../lib/ui.svelte';
  import { ui } from '../lib/ui.svelte';
  import { store } from '../lib/state.svelte';
  import { cmd } from '../lib/net/socket';
  import SheetFrame from './Sheet.svelte';

  let { sheet }: { sheet: Extract<Sheet, { kind: 'addsend' }> } = $props();
  let s = $derived(store.state!);
  let track = $derived(s.tracks.find(t => t.id === sheet.track));
  let buses = $derived(s.tracks.filter(t => t.kind === 'bus'));
  let used = $derived(new Set(track?.sends.map(sd => sd.bus) ?? []));

  async function add(bus: number) {
    try { await cmd('addSend', { track: sheet.track, bus }); ui.sheet = null; } catch { /* toasted */ }
  }
</script>

<SheetFrame title="Send {track?.name ?? ''} to">
  <div class="plist">
    {#each buses as b (b.id)}
      <button class="pitem" style="--tc:{b.color}" disabled={used.has(b.id)} onclick={() => add(b.id)}>
        <i></i><span class="t"><b>{b.name}</b><small>{used.has(b.id) ? `already has a send from ${track?.name}` : b.chain.map(p => p.name).join(' → ') || 'no effects'}</small></span>
        <span class="badge">bus</span>
      </button>
    {/each}
    <button class="pitem" onclick={() => (ui.sheet = { kind: 'add', trackKind: 'bus', sendFrom: sheet.track })}>
      <span class="t"><b>＋ New bus…</b><small>Create a bus and send {track?.name} to it</small></span>
    </button>
  </div>
  <p class="note">New sends start at −12 dB, post-fader.</p>
</SheetFrame>

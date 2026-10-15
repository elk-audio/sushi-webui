<script lang="ts">
  import type { Sheet } from '../lib/ui.svelte';
  import { ui } from '../lib/ui.svelte';
  import { store } from '../lib/state.svelte';
  import { cmd } from '../lib/net/socket';
  import { ROLE_ADD } from '../lib/labels';
  import { allTracks } from '../lib/state.svelte';
  import SheetFrame from './Sheet.svelte';

  let { sheet }: { sheet: Extract<Sheet, { kind: 'browse' }> } = $props();
  let s = $derived(store.state!);
  let track = $derived(allTracks(s).find(t => t.id === sheet.track));
  let q = $state('');
  let adding = $state('');
  const GROUPS: [string, string][] = [['internal', 'Built into Sushi'], ['lv2', 'LV2'], ['vst3', 'VST3'], ['vst2', 'VST2']];
  let items = $derived(s.catalog.filter(c => c.role === sheet.section && (!q || (c.name + ' ' + c.vendor).toLowerCase().includes(q.toLowerCase()))));

  async function add(id: string) {
    adding = id;
    try {
      await cmd('addPlugin', { track: sheet.track, catalogId: id, section: sheet.section });
      ui.sheet = null;
    } catch { /* toasted */ } finally { adding = ''; }
  }
</script>

<SheetFrame title="Add {ROLE_ADD[sheet.section]} to {track?.name ?? 'track'}">
  <input type="search" id="browse-q" placeholder="Search plugins" aria-label="Search plugins" bind:value={q} />
  <div class="plist">
    {#each GROUPS as [ty, label] (ty)}
      {@const xs = items.filter(c => c.type === ty)}
      {#if xs.length}
        <h6>{label}</h6>
        {#each xs as c (c.id)}
          <button class="pitem" disabled={!!adding} onclick={() => add(c.id)}>
            <span class="t"><b>{c.name}</b><small>{c.vendor ? c.vendor + ' · ' : ''}{c.uid}</small></span>
            {#if adding === c.id}<div class="spin" style="margin:0"></div>{:else}<span class="badge">{c.type}</span>{/if}
          </button>
        {/each}
      {/if}
    {/each}
    {#if !items.length}<p class="note">No plugins match.</p>{/if}
  </div>
</SheetFrame>

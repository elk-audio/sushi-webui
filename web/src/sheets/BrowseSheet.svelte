<script lang="ts">
  import type { Sheet } from '../lib/ui.svelte';
  import { ui } from '../lib/ui.svelte';
  import { store } from '../lib/state.svelte';
  import { cmd } from '../lib/net/socket';
  import { ROLE_ADD } from '../lib/labels';
  import { allTracks } from '../lib/state.svelte';
  import type { CatalogEntry } from '../lib/types';
  import SheetFrame from './Sheet.svelte';

  let { sheet }: { sheet: Extract<Sheet, { kind: 'browse' }> } = $props();
  let s = $derived(store.state!);
  let track = $derived(allTracks(s).find(t => t.id === sheet.track));
  let q = $state('');
  let adding = $state('');
  let vendor = $state('');    // '' = top level; then vendor; then vendor + category
  let category = $state('');
  let ofRole = $derived(s.catalog.filter(c => c.role === sheet.section));
  let searching = $derived(q.trim() !== '');
  let found = $derived(ofRole.filter(c => (c.name + ' ' + c.vendor + ' ' + c.category).toLowerCase().includes(q.trim().toLowerCase())));
  const uniq = (xs: string[]) => [...new Set(xs)];
  let vendors = $derived(uniq(ofRole.map(c => c.vendor)));
  let categories = $derived(uniq(ofRole.filter(c => c.vendor === vendor).map(c => c.category)));
  let items = $derived(ofRole.filter(c => c.vendor === vendor && c.category === category));
  const count = (f: (c: CatalogEntry) => boolean) => ofRole.filter(f).length;

  function back() { if (category) category = ''; else vendor = ''; }

  async function add(id: string) {
    adding = id;
    try {
      await cmd('addPlugin', { track: sheet.track, catalogId: id, section: sheet.section });
      ui.sheet = null;
    } catch { /* toasted */ } finally { adding = ''; }
  }
</script>

{#snippet plugin(c: CatalogEntry)}
  <button class="pitem" disabled={!!adding} onclick={() => add(c.id)}>
    <span class="t"><b>{c.name}</b>{#if searching}<small>{c.vendor} › {c.category}</small>{/if}</span>
    {#if adding === c.id}<div class="spin" style="margin:0"></div>{:else}<span class="badge">{c.type}</span>{/if}
  </button>
{/snippet}

<SheetFrame title="Add {ROLE_ADD[sheet.section]} to {track?.name ?? 'track'}">
  <input type="search" id="browse-q" placeholder="Search plugins" aria-label="Search plugins" bind:value={q} />
  <div class="plist">
    {#if searching}
      {#each found as c (c.id)}{@render plugin(c)}{/each}
      {#if !found.length}<p class="note">No plugins match.</p>{/if}
    {:else}
      {#if vendor}
        <nav class="crumbs" aria-label="Plugin location">
          <button class="ghost-btn" onclick={back}>‹ Back</button>
          <span><button onclick={() => { vendor = ''; category = ''; }}>All</button> › {#if category}<button onclick={() => (category = '')}>{vendor}</button> › {category}{:else}{vendor}{/if}</span>
        </nav>
      {/if}
      {#if !vendor}
        {#each vendors as v (v)}
          <button class="pitem" onclick={() => (vendor = v)}><span class="t"><b>{v}</b></span><span class="badge">{count(c => c.vendor === v)}</span><span aria-hidden="true">›</span></button>
        {/each}
      {:else if !category}
        {#each categories as cat (cat)}
          <button class="pitem" onclick={() => (category = cat)}><span class="t"><b>{cat}</b></span><span class="badge">{count(c => c.vendor === vendor && c.category === cat)}</span><span aria-hidden="true">›</span></button>
        {/each}
      {:else}
        {#each items as c (c.id)}{@render plugin(c)}{/each}
      {/if}
      {#if !ofRole.length}<p class="note">No {ROLE_ADD[sheet.section]}s are installed.</p>{/if}
    {/if}
  </div>
</SheetFrame>

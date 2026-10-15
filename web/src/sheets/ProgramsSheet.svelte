<script lang="ts">
  import type { Sheet } from '../lib/ui.svelte';
  import { ui } from '../lib/ui.svelte';
  import { store, allTracks } from '../lib/state.svelte';
  import { act } from '../lib/net/socket';
  import SheetFrame from './Sheet.svelte';

  let { sheet }: { sheet: Extract<Sheet, { kind: 'programs' }> } = $props();
  let proc = $derived(allTracks(store.state).flatMap(t => t.chain).find(p => p.id === sheet.proc));
  function pick(n: number) {
    if (!proc) return;
    proc.program = n;
    act('setProgram', { proc: proc.id, program: n });
    ui.sheet = null;
  }
</script>

<SheetFrame title="{proc?.name ?? ''} programs">
  <div class="plist">
    {#each proc?.programs ?? [] as name, n}
      <button class="pitem" aria-current={n === proc?.program} onclick={() => pick(n)}>
        <span class="t"><b style={n === proc?.program ? 'color:var(--accent)' : ''}>{name}</b></span>
        {#if n === proc?.program}<span class="badge">current</span>{/if}
      </button>
    {/each}
  </div>
</SheetFrame>

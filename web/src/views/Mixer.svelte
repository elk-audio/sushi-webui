<script lang="ts">
  import { store } from '../lib/state.svelte';
  import { ui } from '../lib/ui.svelte';
  import Strip from './Strip.svelte';
  import Row from './Row.svelte';
  import EmptyState from './EmptyState.svelte';

  let s = $derived(store.state!);
  let regs = $derived(s.tracks.filter(t => t.kind !== 'bus'));
  let buses = $derived(s.tracks.filter(t => t.kind === 'bus'));
  const add = () => { ui.sheet = { kind: 'add', trackKind: 'audio' }; };
</script>

{#if ui.size === 'phone'}
  {#if s.tracks.length === 0}
    <EmptyState />
  {:else}
    <div class="rows">
      {#if s.ui.advanced && s.pre}<Row track={s.pre} />{/if}
      {#each regs as t (t.id)}<Row track={t} />{/each}
      <button class="add-row" onclick={add}>＋ Add track or bus</button>
      {#if buses.length}
        <div class="rowdiv">BUSES</div>
        {#each buses as t (t.id)}<Row track={t} />{/each}
      {/if}
    </div>
  {/if}
{:else}
  <div class="mixer">
    <div class="strips">
      {#if s.tracks.length === 0}
        <div style="flex:1"><EmptyState /></div>
      {:else}
        {#if s.ui.advanced && s.pre}<Strip track={s.pre} /><div class="sdiv">TRACKS</div>{/if}
        {#each regs as t (t.id)}<Strip track={t} />{/each}
        <button class="add-strip" onclick={add}><span>＋</span>Add track<br />or bus</button>
        {#if buses.length}
          <div class="sdiv">BUSES</div>
          {#each buses as t (t.id)}<Strip track={t} />{/each}
        {/if}
      {/if}
    </div>
    {#if s.master}<Strip track={s.master} />{/if}
  </div>
{/if}

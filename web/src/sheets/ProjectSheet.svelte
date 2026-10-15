<script lang="ts">
  import { ui } from '../lib/ui.svelte';
  import { store, toast } from '../lib/state.svelte';
  import { cmd, act } from '../lib/net/socket';
  import SheetFrame from './Sheet.svelte';

  let s = $derived(store.state!);
  let saveAs = $state('');
  let confirmOpen = $state('');

  async function save(name?: string) {
    try {
      await cmd('saveProject', name ? { name } : {});
      toast(`Saved ${name ?? s.project.name}`);
      ui.sheet = null;
    } catch { /* toasted */ }
  }
  async function open(name: string) {
    if (s.project.dirty && confirmOpen !== name) { confirmOpen = name; return; }
    try { await cmd('loadProject', { name }); ui.sheet = null; } catch { /* toasted */ }
  }
</script>

<SheetFrame title="Project">
  <div class="plist">
    <button class="pitem" onclick={() => save()}><span class="t"><b>Save “{s.project.name}”</b><small>{s.project.dirty ? 'unsaved changes' : 'no changes since last save'}</small></span></button>
    <div class="pitem" style="gap:8px">
      <input id="save-as" placeholder="Save as…" bind:value={saveAs} style="flex:1"
        onkeydown={e => { if (e.key === 'Enter' && saveAs.trim()) save(saveAs.trim()); }} />
      <button class="secondary" disabled={!saveAs.trim()} onclick={() => save(saveAs.trim())}>Save</button>
    </div>
    <h6>Open</h6>
    {#each s.project.list as name (name)}
      <button class="pitem" onclick={() => open(name)}>
        <span class="t"><b>{name}</b>{#if confirmOpen === name}<small>Unsaved changes will be lost. Tap again to open.</small>{/if}</span>
        {#if name === s.project.name}<span class="badge">current</span>{/if}
      </button>
    {:else}
      <p class="note">No saved projects yet.</p>
    {/each}
    <h6>Advanced</h6>
    <button class="pitem" aria-pressed={s.ui.advanced} onclick={() => act('setAdvanced', { on: !s.ui.advanced })}>
      <span class="t"><b>Show Input and Output tracks</b><small>Effects on all inputs before the tracks, and on all outputs after them</small></span>
      <span class="badge">{s.ui.advanced ? 'On' : 'Off'}</span>
    </button>
  </div>
  <p class="note">Sushi {s.sushi.version} · {s.sushi.samplerate / 1000} kHz · {s.sushi.inputs} in / {s.sushi.outputs} out</p>
</SheetFrame>

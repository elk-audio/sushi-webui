<script lang="ts">
  import { ui } from '../lib/ui.svelte';
  import { store } from '../lib/state.svelte';
  import { cmd } from '../lib/net/socket';
  import { ICON } from '../lib/icons';

  let projects = $derived(store.state?.project.list ?? []);
  let listing = $state(false);
  let loading = $state('');

  async function resume(name: string) {
    loading = name;
    try { await cmd('loadProject', { name }); } catch { /* toasted */ }
    loading = '';
  }
</script>

<div class="empty">
  <h2>Start with a track</h2>
  <p>Sushi is running with no tracks. Pick what you want to play through it.</p>
  <div class="kindcards">
    <button class="kindcard" onclick={() => (ui.sheet = { kind: 'add', trackKind: 'audio' })}>{@html ICON.audio}<b>Audio track</b><span>Guitar, mic or line input through effects</span></button>
    <button class="kindcard" onclick={() => (ui.sheet = { kind: 'add', trackKind: 'midi' })}>{@html ICON.midi}<b>MIDI track</b><span>A synth or sampler played from a keyboard</span></button>
  </div>
  {#if projects.length}
    {#if listing}
      <div class="resume plist" role="group" aria-label="Saved projects">
        <h6>Resume a saved project</h6>
        {#each projects as name (name)}
          <button class="pitem" disabled={!!loading} onclick={() => resume(name)}>
            <span class="t"><b>{name}</b>{#if loading === name}<small>Loading…</small>{/if}</span>
          </button>
        {/each}
      </div>
    {:else}
      <button class="resume-link" aria-expanded="false" onclick={() => (listing = true)}>…or resume a saved project</button>
    {/if}
  {/if}
</div>

<script lang="ts">
  import { ui } from '../lib/ui.svelte';
  import { store } from '../lib/state.svelte';
  import AddSheet from './AddSheet.svelte';
  import BrowseSheet from './BrowseSheet.svelte';
  import AddSendSheet from './AddSendSheet.svelte';
  import ProgramsSheet from './ProgramsSheet.svelte';
  import ProjectSheet from './ProjectSheet.svelte';
  import FocusSheet from './FocusSheet.svelte';

  $effect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape' && ui.sheet) ui.sheet = null; };
    addEventListener('keydown', onKey);
    return () => removeEventListener('keydown', onKey);
  });
</script>

{#if ui.sheet && store.state}
  {#if ui.sheet.kind === 'add'}<AddSheet sheet={ui.sheet} />
  {:else if ui.sheet.kind === 'browse'}<BrowseSheet sheet={ui.sheet} />
  {:else if ui.sheet.kind === 'addsend'}<AddSendSheet sheet={ui.sheet} />
  {:else if ui.sheet.kind === 'programs'}<ProgramsSheet sheet={ui.sheet} />
  {:else if ui.sheet.kind === 'project'}<ProjectSheet />
  {:else if ui.sheet.kind === 'focus'}<FocusSheet sheet={ui.sheet} />
  {/if}
{/if}

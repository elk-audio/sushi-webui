<script lang="ts">
  import type { Snippet } from 'svelte';
  import { ui } from '../lib/ui.svelte';

  let { title, children, footer }: { title: string; children: Snippet; footer?: Snippet } = $props();
  const close = () => { ui.sheet = null; };
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="scrim" role="presentation" onclick={e => { if (e.target === e.currentTarget) close(); }}>
  <div class="sheet" role="dialog" aria-label={title} tabindex="-1">
    <div class="sheet-h"><h2>{title}</h2><button class="x" aria-label="Close" onclick={close}>✕</button></div>
    <div class="sheet-b">{@render children()}</div>
    {#if footer}<div class="sheet-f">{@render footer()}</div>{/if}
  </div>
</div>

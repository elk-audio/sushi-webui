<script lang="ts">
  import type { Sheet } from '../lib/ui.svelte';
  import { ui } from '../lib/ui.svelte';
  import HFader from '../lib/widgets/HFader.svelte';
  import SheetFrame from './Sheet.svelte';

  let { sheet }: { sheet: Extract<Sheet, { kind: 'focus' }> } = $props();
  let ctl = $derived(sheet.ctl);
</script>

<SheetFrame title={ctl.label}>
  <div class="focus">
    <div class="big">{ctl.fmt()}</div>
    <HFader {ctl} />
    <div class="hint"><span>Drag along the bar · Shift or a second finger for fine</span><span>Double-tap to reset</span></div>
  </div>
  {#snippet footer()}
    {#if ctl.def !== undefined}<button class="secondary" onclick={() => ctl.set(ctl.def!)}>Reset</button>{/if}
    <button class="primary" onclick={() => (ui.sheet = null)}>Done</button>
  {/snippet}
</SheetFrame>

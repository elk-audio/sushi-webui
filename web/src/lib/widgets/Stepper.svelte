<script lang="ts">
  import type { Ctl } from '../types';
  import { clamp } from '../util';

  let { ctl }: { ctl: Ctl } = $props();
  const step = (d: number) => ctl.set(clamp(ctl.get() + d / (ctl.steps || 100)));
</script>

<div class="pctl">
  <div class="stepper">
    <button onclick={() => step(-1)} aria-label="Decrease {ctl.label}" disabled={ctl.get() <= 0}>−</button>
    <span class="sv">{ctl.fmt()}</span>
    <button onclick={() => step(1)} aria-label="Increase {ctl.label}" disabled={ctl.get() >= 1}>+</button>
  </div>
  <span class="lab">{ctl.label}</span>
</div>

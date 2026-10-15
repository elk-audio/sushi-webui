<script lang="ts">
  import type { Ctl } from '../types';
  import { drag } from '../drag';

  let {
    ctl, size = 44, label = null, mini = false, pinned = false, pickable = false, onTap, onpick,
  }: {
    ctl: Ctl; size?: number; label?: string | null; mini?: boolean; pinned?: boolean;
    pickable?: boolean; onTap?: () => void; onpick?: () => void;
  } = $props();

  const C = 2 * Math.PI * 40, SWEEP = 0.75 * C;
  let v = $derived(ctl.get());
  let arc = $derived.by(() => {
    let start = 135, len = v * SWEEP;
    if (ctl.bipolar) {
      if (v >= 0.5) { start = 270; len = (v - 0.5) * SWEEP; }
      else { start = 135 + 270 * v; len = (0.5 - v) * SWEEP; }
    }
    return { start, len: Math.max(len, 0.001) };
  });
  let text = $derived(label === null ? ctl.label : label);
  let width = $derived(mini ? 38 : size <= 32 ? 44 : Math.max(size, 56));
</script>

<div
  class="knob" class:mini class:pinned class:pickable
  role="slider" tabindex="0" aria-label={ctl.label} aria-valuenow={Math.round(v * 100)} aria-valuetext={ctl.fmt()}
  style="width:{width}px"
  use:drag={{ ctl, mode: 'knob', onTap, disabled: pickable }}
  onclick={() => { if (pickable) onpick?.(); }}
  onkeydown={() => {}}
>
  <svg width={size} height={size} viewBox="0 0 100 100" aria-hidden="true">
    <circle class="trk" cx="50" cy="50" r="40" fill="none" stroke-width="9" stroke-dasharray="{SWEEP} {C}" transform="rotate(135 50 50)" stroke-linecap="round" />
    <circle class="val" cx="50" cy="50" r="40" fill="none" stroke-width="9" stroke-linecap="round" stroke-dasharray="{arc.len} {C}" transform="rotate({arc.start} 50 50)" />
    <circle class="cap" cx="50" cy="50" r="28" stroke-width="2" />
    <line class="ind" x1="50" y1="50" x2="50" y2="28" transform="rotate({135 + 270 * v - 270} 50 50)" />
  </svg>
  {#if text}<span class="lab">{text}</span>{/if}
  <span class="v">{ctl.fmt()}</span>
</div>

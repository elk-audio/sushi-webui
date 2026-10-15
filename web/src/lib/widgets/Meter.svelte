<script lang="ts">
  import { registerMeter } from '../render/meterLoop';
  import { clearClip } from '../net/meters';

  let { slot, orient = 'v', cls = '', title = '' }: { slot: number; orient?: 'v' | 'h'; cls?: string; title?: string } = $props();

  function meter(node: HTMLCanvasElement, s: number) {
    return registerMeter(node, s, orient === 'v');
  }
</script>

<canvas class="meter {cls}" use:meter={slot} {title} onpointerdown={() => clearClip(slot)}></canvas>

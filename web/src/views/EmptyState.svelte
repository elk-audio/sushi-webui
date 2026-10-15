<script lang="ts">
  import { ui } from '../lib/ui.svelte';
  import { ICON } from '../lib/icons';
</script>

<div class="empty">
  <h2>Start with a track</h2>
  <p>Sushi is running with no tracks. Pick what you want to play through it.</p>
  <div class="kindcards">
    <button class="kindcard" onclick={() => (ui.sheet = { kind: 'add', trackKind: 'audio' })}>{@html ICON.audio}<b>Audio track</b><span>Guitar, mic or line input through effects</span></button>
    <button class="kindcard" onclick={() => (ui.sheet = { kind: 'add', trackKind: 'midi' })}>{@html ICON.midi}<b>MIDI track</b><span>A synth or sampler played from a keyboard</span></button>
  </div>
</div>

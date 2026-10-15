<script lang="ts">
  import type { Ctl, Processor, Track } from '../lib/types';
  import { store } from '../lib/state.svelte';
  import { ui, go, trackKey } from '../lib/ui.svelte';
  import { act } from '../lib/net/socket';
  import { clips } from '../lib/net/meters';
  import { drag } from '../lib/drag';
  import { ICON } from '../lib/icons';

  let { track, proc }: { track: Track | null; proc: Processor | null } = $props();
  let s = $derived(store.state);
  let phone = $derived(ui.size === 'phone');

  // Master clip indicator (clips live outside Svelte reactivity; poll them).
  let masterClip = $state(false);
  $effect(() => {
    const id = setInterval(() => {
      const m = store.state?.master?.meter ?? -1;
      masterClip = m >= 0 && (clips[m * 2] === 1 || clips[m * 2 + 1] === 1);
    }, 300);
    return () => clearInterval(id);
  });

  let tempoTimer: ReturnType<typeof setTimeout> | undefined;
  const tempoCtl: Ctl = {
    key: 'tempo', label: 'Tempo',
    get: () => ((store.state?.transport.tempo ?? 120) - 20) / 280,
    set: v => {
      if (!store.state) return;
      store.state.transport.tempo = Math.round((20 + v * 280) * 10) / 10;
      clearTimeout(tempoTimer);
      tempoTimer = setTimeout(() => act('transport', { tempo: store.state!.transport.tempo }), 80);
    },
    fmt: () => (store.state?.transport.tempo ?? 120).toFixed(1),
    def: 100 / 280,
  };
  const openProject = () => { ui.sheet = { kind: 'project' }; };
</script>

<header class="topbar">
  {#if phone}
    {#if ui.route.view === 'mixer' || !track}
      <button class="proj" onclick={openProject} aria-label="Project menu">{s?.project.name ?? 'Sushi'}
        {#if s?.project.dirty}<span class="dirty"></span>{/if}{#if masterClip}<span class="clipdot"></span>{/if}</button>
    {:else if ui.route.view === 'track'}
      <button class="backbtn" onclick={() => go({ view: 'mixer' })}>{@html ICON.back}Mixer</button>
    {:else if s}
      <button class="backbtn" onclick={() => go({ view: 'track', t: trackKey(s, track) })}>{@html ICON.back}{track.name}</button>
    {/if}
  {:else}
    <button class="proj" onclick={openProject} aria-label="Project menu">{s?.project.name ?? 'Sushi'}
      {#if s?.project.dirty}<span class="dirty" title="Unsaved changes"></span>{/if}{#if masterClip}<span class="clipdot" title="Master clipped"></span>{/if}</button>
    <span class="crumbs sep" aria-hidden="true">/</span>
    <nav class="crumbs">
      <button class:here={ui.route.view === 'mixer'} onclick={() => go({ view: 'mixer' })}>Mixer</button>
      {#if track && s}
        <span class="sep">›</span>
        <button class:here={ui.route.view === 'track'} onclick={() => go({ view: 'track', t: trackKey(s, track) })}>{track.name}</button>
      {/if}
      {#if proc}<span class="sep">›</span><span class="here">{proc.name}</span>{/if}
    </nav>
  {/if}
  <div class="spacer"></div>
  {#if s}
    <div class="transport">
      {#if !phone}
        <div class="tempo num" role="slider" tabindex="0" aria-label="Tempo" aria-valuenow={s.transport.tempo} use:drag={{ ctl: tempoCtl, mode: 'num' }}>
          {s.transport.tempo.toFixed(1)}<small>BPM</small>
        </div>
      {/if}
      <button class="tbtn" class:on={s.transport.playing} aria-label={s.transport.playing ? 'Playing' : 'Play'} aria-pressed={s.transport.playing}
        onclick={() => act('transport', { playing: !s.transport.playing })}>{@html ICON.play}</button>
      {#if !phone}<button class="tbtn" aria-label="Stop" onclick={() => act('transport', { playing: false })}>{@html ICON.stop}</button>{/if}
    </div>
    <span class="cpu num">{Math.round(s.cpu * 100)}%{phone ? '' : ' CPU'}</span>
  {/if}
  <span class="conn" class:lost={!store.sockOpen} class:down={store.sockOpen && s && !s.sushi.connected}
    title={!store.sockOpen ? 'Reconnecting to sushi-webui' : s && !s.sushi.connected ? 'Sushi not reachable' : 'Connected to Sushi'}></span>
</header>

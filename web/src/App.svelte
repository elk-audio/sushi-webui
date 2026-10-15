<script lang="ts">
  import { store } from './lib/state.svelte';
  import { ui, findTrack, go } from './lib/ui.svelte';
  import TopBar from './views/TopBar.svelte';
  import Mixer from './views/Mixer.svelte';
  import TrackView from './views/TrackView.svelte';
  import PluginView from './views/PluginView.svelte';
  import MasterDock from './views/MasterDock.svelte';
  import TrackDock from './views/TrackDock.svelte';
  import Sheets from './sheets/Sheets.svelte';

  let el: HTMLDivElement;

  $effect(() => {
    const measure = () => {
      const w = el.clientWidth, h = el.clientHeight;
      ui.size = w < 600 ? 'phone' : w < 1024 ? 'tablet' : 'desktop';
      ui.short = h < 520 && ui.size !== 'phone';
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  });

  let track = $derived(ui.route.view !== 'mixer' ? findTrack(store.state, ui.route.t) : null);
  let proc = $derived(ui.route.view === 'plugin' && track ? (track.chain.find(p => p.id === (ui.route as { p: number }).p) ?? null) : null);

  // Fall back when the routed track/plugin disappears (deleted, Sushi restarted).
  $effect(() => {
    if (!store.state) return;
    const r = ui.route;
    if (r.view !== 'mixer' && !track) go({ view: 'mixer' });
    else if (r.view === 'plugin' && !proc) go({ view: 'track', t: r.t });
  });

  let offline = $derived(!store.sockOpen || (store.state !== null && !store.state.sushi.connected));
</script>

<div class="app is-{ui.size}" class:is-short={ui.short} class:offline bind:this={el}>
  <TopBar {track} {proc} />
  {#if !store.sockOpen}
    <div class="banner" role="status">Lost connection to sushi-webui · reconnecting…</div>
  {:else if store.state && !store.state.sushi.connected}
    <div class="banner err" role="status">Sushi isn't reachable from sushi-webui · retrying…</div>
  {/if}
  <main class="content">
    {#if !store.state}
      <div class="waiting">Connecting to sushi-webui…</div>
    {:else if ui.route.view === 'mixer'}
      <Mixer />
    {:else if ui.route.view === 'track' && track}
      <TrackView {track} />
    {:else if ui.route.view === 'plugin' && track && proc}
      <PluginView {track} {proc} />
    {/if}
  </main>
  {#if store.state && ui.size === 'phone'}
    {#if ui.route.view === 'mixer' && store.state.master}
      <MasterDock master={store.state.master} />
    {:else if ui.route.view === 'track' && track && ui.showMix}
      <TrackDock {track} />
    {/if}
  {/if}
  <Sheets />
  {#if store.toast}<div class="toast" role="status">{store.toast}</div>{/if}
  {#if store.busy > 0}<div class="busy" aria-label="Working"><div class="spin"></div></div>{/if}
</div>

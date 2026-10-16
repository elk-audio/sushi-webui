<script lang="ts">
  import type { Role, Track } from '../lib/types';
  import { store } from '../lib/state.svelte';
  import { ui, go, navOrder, toggleMix, trackKey } from '../lib/ui.svelte';
  import { act, cmd } from '../lib/net/socket';
  import { ROLE_ADD, ROLE_LABEL, isFixed, kindHtml, routeIn, sectionsFor } from '../lib/labels';
  import Meter from '../lib/widgets/Meter.svelte';
  import PluginCard from './PluginCard.svelte';
  import SendCard from './SendCard.svelte';
  import Inspector from './Inspector.svelte';

  let { track }: { track: Track } = $props();
  let s = $derived(store.state!);
  let phone = $derived(ui.size === 'phone');
  let key = $derived(trackKey(s, track));
  let order = $derived(navOrder(s));
  let idx = $derived(order.indexOf(key));
  let prev = $derived(idx > 0 ? order[idx - 1] : undefined);
  let next = $derived(idx >= 0 && idx < order.length - 1 ? order[idx + 1] : undefined);
  let sections = $derived(sectionsFor(track).map(sec => ({ sec, items: track.chain.filter(p => p.role === sec) })));
  let [inL, inV] = $derived(routeIn(s, track));

  // Phone: collapsed cards (first card of each section starts expanded).
  // Click the name to rename the track (Sushi tracks can't be renamed; the backend keeps the label).
  let renaming = $state(false);
  let draft = $state('');
  function startRename() {
    if (isFixed(track)) return;
    draft = track.name;
    renaming = true;
  }
  async function commitRename() {
    if (!renaming) return;
    renaming = false;
    const name = draft.trim();
    if (!name || name === track.name) return;
    try { await cmd('renameTrack', { track: track.id, name }); } catch { /* toasted */ }
  }
  function focusSelect(el: HTMLInputElement) { el.focus(); el.select(); }

  let collapsed = $state<Record<number, boolean>>({});
  const isCollapsed = (id: number, first: boolean) => collapsed[id] ?? !first;

  function stepTrack(d: number) {
    const t = d < 0 ? prev : next;
    if (t !== undefined) go({ view: 'track', t });
  }
  $effect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (ui.sheet || (e.target as HTMLElement).closest('input,select,textarea')) return;
      if (e.key === ']') stepTrack(1);
      else if (e.key === '[') stepTrack(-1);
    };
    addEventListener('keydown', onKey);
    return () => removeEventListener('keydown', onKey);
  });

  // Swipe between tracks on touch.
  let swipe: { x: number; y: number; id: number } | null = null;
  function swDown(e: PointerEvent) {
    if (!phone || (e.target as HTMLElement).closest('[role="slider"],.handle')) return;
    swipe = { x: e.clientX, y: e.clientY, id: e.pointerId };
  }
  function swUp(e: PointerEvent) {
    if (!swipe || e.pointerId !== swipe.id) return;
    const dx = e.clientX - swipe.x, dy = e.clientY - swipe.y;
    swipe = null;
    if (Math.abs(dx) > 70 && Math.abs(dx) > 2 * Math.abs(dy)) stepTrack(dx < 0 ? 1 : -1);
  }

  // Drag-to-reorder within a section → movePlugin with the visible-chain index.
  let reorder: { proc: number; el: HTMLElement; sec: Role; x0: number; y0: number; px: number; py: number; id: number } | null = null;
  function startReorder(e: PointerEvent, el: HTMLElement, proc: number, sec: Role) {
    e.preventDefault();
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    reorder = { proc, el, sec, x0: e.clientX, y0: e.clientY, px: e.clientX, py: e.clientY, id: e.pointerId };
    el.classList.add('drag');
    const move = (ev: PointerEvent) => {
      if (!reorder || ev.pointerId !== reorder.id) return;
      reorder.px = ev.clientX; reorder.py = ev.clientY;
      el.style.transform = phone ? `translateY(${ev.clientY - reorder.y0}px)` : `translateX(${ev.clientX - reorder.x0}px)`;
    };
    const up = (ev: PointerEvent) => {
      const target = ev.currentTarget as HTMLElement;
      target.removeEventListener('pointermove', move);
      target.removeEventListener('pointerup', up);
      target.removeEventListener('pointercancel', up);
      finishReorder();
    };
    const h = e.currentTarget as HTMLElement;
    h.addEventListener('pointermove', move);
    h.addEventListener('pointerup', up);
    h.addEventListener('pointercancel', up);
  }
  function finishReorder() {
    const r = reorder;
    reorder = null;
    if (!r) return;
    r.el.classList.remove('drag');
    r.el.style.transform = '';
    const sectionEl = r.el.closest('.section');
    if (!sectionEl) return;
    const siblings = [...sectionEl.querySelectorAll<HTMLElement>('.card[data-proc]')].filter(c => c !== r.el);
    let before: number | null = null;
    for (const c of siblings) {
      const b = c.getBoundingClientRect();
      if (phone ? r.py < b.top + b.height / 2 : r.px < b.left + b.width / 2) { before = +c.dataset.proc!; break; }
    }
    const rest = track.chain.filter(p => p.id !== r.proc).map(p => p.id);
    let index: number;
    if (before !== null) index = rest.indexOf(before);
    else if (siblings.length) index = rest.indexOf(+siblings[siblings.length - 1].dataset.proc!) + 1;
    else return;
    const cur = track.chain.findIndex(p => p.id === r.proc);
    if (index === cur) return;
    // optimistic reorder
    const moved = track.chain.splice(cur, 1)[0];
    track.chain.splice(index, 0, moved);
    act('movePlugin', { proc: r.proc, index });
  }
</script>

<div class="trackview">
  <div class="thead" style="--tc:{track.color}">
    <button class="nav" disabled={prev === undefined} aria-label="Previous track" title="Previous track ( [ )" onclick={() => stepTrack(-1)}>‹</button>
    <i></i>
    {#if renaming}
      <input class="tn-edit" aria-label="Track name" maxlength="40" bind:value={draft} use:focusSelect onblur={commitRename}
        onkeydown={e => { e.stopPropagation(); if (e.key === 'Enter') commitRename(); else if (e.key === 'Escape') renaming = false; }} />
    {:else if isFixed(track)}
      <span class="tn">{track.name}</span>
    {:else}
      <button class="tn" title="Rename track" onclick={startRename}>{track.name}</button>
    {/if}
    <button class="nav" disabled={next === undefined} aria-label="Next track" title="Next track ( ] )" onclick={() => stepTrack(1)}>›</button>
    <span class="kind">{@html kindHtml(s, track)}</span>
    <div class="spacer"></div>
    {#if track.meter >= 0}<Meter slot={track.meter} orient="h" title="{track.name} level" />{/if}
    <button class="mixbtn" aria-pressed={ui.showMix} onclick={toggleMix}>{phone ? 'Mix' : 'Mix & routing'}</button>
  </div>
  <div class="tbody">
    <div class="chainwrap" onpointerdown={swDown} onpointerup={swUp} role="presentation">
      <div class="chain">
        {#if ui.showMix}<div class="io"><b>{inL}</b><span>{inV}</span><span class="arrow">→</span></div>{/if}
        {#each sections as { sec, items } (sec)}
          <div class="section">
            <h3>{ROLE_LABEL[sec]}</h3>
            <div class="cards">
              {#each items as p, i (p.id)}
                <PluginCard proc={p} tkey={key} collapsed={isCollapsed(p.id, i === 0)}
                  ontoggle={() => (collapsed[p.id] = !isCollapsed(p.id, i === 0))}
                  onhandle={(e, el) => startReorder(e, el, p.id, sec)} />
              {/each}
              {#if !(sec === 'instrument' && items.length >= 1)}
                <button class="addslot" aria-label="Add {ROLE_ADD[sec]}" onclick={() => (ui.sheet = { kind: 'browse', track: track.id, section: sec })}>{phone ? '＋ Add ' + ROLE_ADD[sec] : '＋'}</button>
              {/if}
            </div>
          </div>
        {/each}
        {#if track.kind === 'audio' || track.kind === 'midi'}
          <div class="section">
            <h3>Sends</h3>
            <div class="cards">
              {#each track.sends as sd (sd.bus)}<SendCard {track} send={sd} />{/each}
              <button class="addslot" aria-label="Add send" onclick={() => (ui.sheet = { kind: 'addsend', track: track.id })}>{phone ? '＋ Add send' : '＋'}</button>
            </div>
          </div>
        {/if}
        {#if ui.showMix}<div class="io"><span class="arrow">→</span><b>OUT</b><span>{track.output}</span></div>{/if}
      </div>
    </div>
    {#if !phone && ui.showMix}<Inspector {track} />{/if}
  </div>
</div>

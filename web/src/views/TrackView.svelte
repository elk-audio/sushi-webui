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

  // Reorder within a section → movePlugin with the visible-chain index. The backend rejects moves across
  // sections, and the UI never offers them: a drag only considers the cards of the dragged card's section.
  const HOLD_MS = 250; // phone: hold the handle before a drag starts, so scrolling and swiping still work
  let reorder: { proc: number; el: HTMLElement; x0: number; y0: number; px: number; py: number; id: number; raf: number } | null = null;

  async function moveTo(proc: number, index: number) {
    const snapshot = track.chain.map(p => p.id);
    const cur = track.chain.findIndex(p => p.id === proc);
    if (cur < 0 || index === cur) return;
    const moved = track.chain.splice(cur, 1)[0]; // optimistic
    track.chain.splice(index, 0, moved);
    try { await cmd('movePlugin', { proc, index }); }
    catch { // toasted; put the chain back until the next state arrives
      const byId = new Map(track.chain.map(p => [p.id, p]));
      track.chain = snapshot.map(id => byId.get(id)!).filter(Boolean);
    }
  }

  // Where a drop at the pointer would land: the chain index, and the card it goes next to.
  function dropTarget(r: NonNullable<typeof reorder>) {
    const sectionEl = r.el.closest('.section');
    if (!sectionEl) return null;
    const siblings = [...sectionEl.querySelectorAll<HTMLElement>('.card[data-proc]')].filter(c => c !== r.el);
    if (!siblings.length) return null;
    let before: HTMLElement | null = null;
    for (const c of siblings) {
      const b = c.getBoundingClientRect();
      if (phone ? r.py < b.top + b.height / 2 : r.px < b.left + b.width / 2) { before = c; break; }
    }
    const rest = track.chain.filter(p => p.id !== +r.el.dataset.proc!).map(p => p.id);
    const next = before ?? siblings[siblings.length - 1];
    const index = rest.indexOf(+next.dataset.proc!) + (before ? 0 : 1);
    return { index, el: next, after: !before, siblings };
  }
  function markDrop(r: NonNullable<typeof reorder>) {
    const d = dropTarget(r);
    for (const c of document.querySelectorAll('.card.drop-before,.card.drop-after')) c.classList.remove('drop-before', 'drop-after');
    d?.el.classList.add(d.after ? 'drop-after' : 'drop-before');
  }
  // Keep the dragged card under the pointer while the pane scrolls near its edges.
  function autoScroll(r: NonNullable<typeof reorder>) {
    const box = r.el.closest<HTMLElement>('.chainwrap');
    if (box) {
      const b = box.getBoundingClientRect(), edge = 48;
      const dy = phone ? (r.py < b.top + edge ? -12 : r.py > b.bottom - edge ? 12 : 0) : 0;
      const dx = !phone ? (r.px < b.left + edge ? -12 : r.px > b.right - edge ? 12 : 0) : 0;
      if (dx || dy) { box.scrollBy(dx, dy); r.x0 -= dx; r.y0 -= dy; place(r); markDrop(r); }
    }
    r.raf = requestAnimationFrame(() => reorder && autoScroll(reorder));
  }
  const place = (r: NonNullable<typeof reorder>) => {
    r.el.style.transform = phone ? `translateY(${r.py - r.y0}px)` : `translateX(${r.px - r.x0}px)`;
  };

  function startReorder(e: PointerEvent, el: HTMLElement, proc: number) {
    const h = e.currentTarget as HTMLElement;
    const pid = e.pointerId;
    const touch = e.pointerType === 'touch';
    let r: NonNullable<typeof reorder> | null = null;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const begin = () => {
      if (r) return;
      h.setPointerCapture(pid);
      r = reorder = { proc, el, x0: x, y0: y, px: x, py: y, id: pid, raf: 0 };
      el.classList.add('drag');
      r.raf = requestAnimationFrame(() => reorder && autoScroll(reorder));
      navigator.vibrate?.(8);
    };
    let x = e.clientX, y = e.clientY;
    const move = (ev: PointerEvent) => {
      if (ev.pointerId !== pid) return;
      x = ev.clientX; y = ev.clientY;
      if (!r) { // before the hold completes, moving away means the user is scrolling
        if (touch && Math.hypot(x - e.clientX, y - e.clientY) > 8) end(false);
        return;
      }
      r.px = x; r.py = y;
      place(r);
      markDrop(r);
    };
    const end = (drop: boolean) => {
      clearTimeout(timer);
      h.removeEventListener('pointermove', move);
      h.removeEventListener('pointerup', up);
      h.removeEventListener('pointercancel', cancel);
      if (!r) return;
      cancelAnimationFrame(r.raf);
      const fin = r;
      reorder = null;
      fin.el.classList.remove('drag');
      fin.el.style.transform = '';
      for (const c of document.querySelectorAll('.card.drop-before,.card.drop-after')) c.classList.remove('drop-before', 'drop-after');
      if (drop) { const d = dropTarget(fin); if (d) moveTo(proc, d.index); }
    };
    const up = (ev: PointerEvent) => { if (ev.pointerId === pid) end(true); };
    const cancel = (ev: PointerEvent) => { if (ev.pointerId === pid) end(false); };
    h.addEventListener('pointermove', move);
    h.addEventListener('pointerup', up);
    h.addEventListener('pointercancel', cancel);
    if (touch) timer = setTimeout(begin, HOLD_MS);
    else { e.preventDefault(); begin(); }
  }

  // Keyboard: Alt+↑/↓ (Alt+←/→ on desktop) on the focused handle moves the plugin one slot within its section.
  function nudge(proc: number, sec: Role, d: number) {
    const peers = track.chain.filter(p => p.role === sec);
    const k = peers.findIndex(p => p.id === proc), to = peers[k + d];
    if (k < 0 || !to) return;
    moveTo(proc, track.chain.findIndex(p => p.id === to.id));
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
                  onhandle={(e, el) => startReorder(e, el, p.id)}
                  onnudge={d => nudge(p.id, sec, d)} movable={items.length > 1} />
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

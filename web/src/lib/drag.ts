import type { Ctl } from './types';
import { beginDrag, endDrag } from './state.svelte';
import { clamp } from './util';

export type DragMode = 'knob' | 'vfader' | 'hfader' | 'num';
export interface DragOpts { ctl: Ctl; mode: DragMode; onTap?: () => void; disabled?: boolean }

// Pointers down anywhere: a second finger switches the active drag to fine mode.
const down = new Set<number>();
addEventListener('pointerdown', e => down.add(e.pointerId), true);
addEventListener('pointerup', e => down.delete(e.pointerId), true);
addEventListener('pointercancel', e => down.delete(e.pointerId), true);

/** Svelte action: relative drag, fine mode, double-tap reset, wheel and arrow keys. */
export function drag(node: HTMLElement, opts: DragOpts) {
  let o = opts;
  let st: null | { id: number; x0: number; y0: number; v0: number; lx: number; ly: number; captured: boolean; moved: boolean; fine: boolean } = null;
  let lastTap = 0;
  let tapTimer: ReturnType<typeof setTimeout> | undefined;

  const range = () => {
    const r = node.getBoundingClientRect();
    if (o.mode === 'vfader') return Math.max(40, r.height - 20);
    if (o.mode === 'hfader') return Math.max(40, r.width - 16);
    if (o.mode === 'num') return 300;
    return 200;
  };
  const capture = (e: PointerEvent) => {
    if (!st) return;
    st.captured = true;
    node.setPointerCapture(e.pointerId);
    node.classList.add('dragging');
    beginDrag(o.ctl.key);
  };

  function onDown(e: PointerEvent) {
    if (o.disabled || st || e.button > 0) return;
    st = { id: e.pointerId, x0: e.clientX, y0: e.clientY, v0: o.ctl.get(), lx: e.clientX, ly: e.clientY, captured: false, moved: false, fine: e.shiftKey };
    if (o.mode !== 'hfader') { capture(e); e.preventDefault(); }
  }
  function onMove(e: PointerEvent) {
    if (!st || e.pointerId !== st.id) return;
    const dx = e.clientX - st.x0, dy = e.clientY - st.y0;
    if (!st.captured) {
      if (Math.abs(dx) > 6 && Math.abs(dx) > Math.abs(dy)) { capture(e); st.x0 = e.clientX; st.y0 = e.clientY; st.v0 = o.ctl.get(); }
      else if (Math.abs(dy) > 8) st = null;               // vertical: let the page scroll
      return;
    }
    const fine = e.shiftKey || down.size > 1;
    if (fine !== st.fine) { st.fine = fine; st.v0 = o.ctl.get(); st.x0 = e.clientX; st.y0 = e.clientY; }
    st.lx = e.clientX; st.ly = e.clientY;
    const delta = o.mode === 'hfader' ? (e.clientX - st.x0) / range() : -(e.clientY - st.y0) / range();
    if (Math.abs(e.clientX - st.x0) + Math.abs(e.clientY - st.y0) > 3) st.moved = true;
    o.ctl.set(clamp(st.v0 + delta * (st.fine ? 0.1 : 1)));
  }
  function onUp(e: PointerEvent) {
    if (!st || e.pointerId !== st.id) return;
    const s = st;
    st = null;
    node.classList.remove('dragging');
    if (s.captured) endDrag(o.ctl.key);
    if (s.moved || e.type !== 'pointerup') return;
    const now = performance.now();
    if (now - lastTap < 320) {
      lastTap = 0;
      clearTimeout(tapTimer);
      if (o.ctl.def !== undefined) o.ctl.set(o.ctl.def);
      return;
    }
    lastTap = now;
    if (o.onTap) { const f = o.onTap; tapTimer = setTimeout(f, 330); }
  }
  function onWheel(e: WheelEvent) {
    if (o.disabled || (o.mode !== 'knob' && o.mode !== 'num')) return;
    e.preventDefault();
    o.ctl.set(clamp(o.ctl.get() - Math.sign(e.deltaY) * (e.shiftKey ? 0.005 : 0.02)));
  }
  function onKey(e: KeyboardEvent) {
    const step = ({ ArrowUp: 1, ArrowRight: 1, ArrowDown: -1, ArrowLeft: -1 } as Record<string, number>)[e.key];
    if (!step || o.disabled) return;
    e.preventDefault();
    const inc = o.ctl.steps ? 1 / o.ctl.steps : e.shiftKey ? 0.005 : 0.02;
    o.ctl.set(clamp(o.ctl.get() + step * inc));
  }

  node.addEventListener('pointerdown', onDown);
  node.addEventListener('pointermove', onMove);
  node.addEventListener('pointerup', onUp);
  node.addEventListener('pointercancel', onUp);
  node.addEventListener('wheel', onWheel, { passive: false });
  node.addEventListener('keydown', onKey);
  return {
    update(n: DragOpts) { o = n; },
    destroy() {
      clearTimeout(tapTimer);
      if (st?.captured) endDrag(o.ctl.key);
      node.removeEventListener('pointerdown', onDown);
      node.removeEventListener('pointermove', onMove);
      node.removeEventListener('pointerup', onUp);
      node.removeEventListener('pointercancel', onUp);
      node.removeEventListener('wheel', onWheel);
      node.removeEventListener('keydown', onKey);
    },
  };
}

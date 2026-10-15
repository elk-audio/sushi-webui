import type { Param, State, Track } from './types';

export const store = $state({
  state: null as State | null,
  sockOpen: false,
  clientId: '',
  rev: 0,
  busy: 0,
  toast: '',
});

// procId:paramId → the reactive Param object inside store.state
const index = new Map<string, Param>();
// params being dragged locally: incoming values are parked until the gesture ends
const dragging = new Set<string>();
const parked = new Map<string, number>();

export const pkey = (p: number, k: number) => `${p}:${k}`;

export function allTracks(s: State | null): Track[] {
  if (!s) return [];
  const out = [...s.tracks];
  if (s.pre) out.push(s.pre);
  if (s.master) out.push(s.master);
  return out;
}

export function setState(s: State, rev: number) {
  store.state = s;
  store.rev = rev;
  index.clear();
  for (const t of allTracks(store.state)) {
    if (t.gain) index.set(pkey(t.id, t.gain.id), t.gain);
    if (t.pan) index.set(pkey(t.id, t.pan.id), t.pan);
    for (const pr of t.chain) for (const p of pr.params) index.set(pkey(pr.id, p.id), p);
  }
}

export function applyParams(u: [number, number, number, string][]) {
  for (const [p, k, n, txt] of u) {
    const key = pkey(p, k);
    const prm = index.get(key);
    if (!prm) continue;
    prm.txt = txt;
    if (dragging.has(key)) { parked.set(key, n); continue; }
    if (prm.n !== n) prm.n = n;
  }
}

export function beginDrag(key: string) { dragging.add(key); }
export function endDrag(key: string) {
  dragging.delete(key);
  const v = parked.get(key);
  parked.delete(key);
  const prm = index.get(key);
  if (prm && v !== undefined) prm.n = v;
}
export const isDragging = (key: string) => dragging.has(key);

let toastTimer: ReturnType<typeof setTimeout> | undefined;
export function toast(msg: string) {
  store.toast = msg;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { store.toast = ''; }, 2600);
}

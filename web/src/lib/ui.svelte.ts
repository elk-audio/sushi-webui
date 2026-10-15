import type { Role, State, Track } from './types';
import type { Ctl } from './types';

export type Route =
  | { view: 'mixer' }
  | { view: 'track'; t: string }
  | { view: 'plugin'; t: string; p: number };

export type Sheet =
  | { kind: 'add'; trackKind: 'audio' | 'midi' | 'bus'; sendFrom?: number }
  | { kind: 'browse'; track: number; section: Role }
  | { kind: 'addsend'; track: number }
  | { kind: 'programs'; proc: number }
  | { kind: 'project' }
  | { kind: 'focus'; ctl: Ctl };

function loadShowMix(): boolean {
  try { return localStorage.getItem('sushi.showMix') === '1'; } catch { return false; }
}

export function parseHash(h: string = location.hash): Route {
  const parts = h.replace(/^#\/?/, '').split('/').filter(Boolean);
  if (parts[0] === 'track' && parts[1]) {
    if (parts[2] === 'plugin' && parts[3]) return { view: 'plugin', t: parts[1], p: +parts[3] };
    return { view: 'track', t: parts[1] };
  }
  return { view: 'mixer' };
}
export function routeHash(r: Route): string {
  if (r.view === 'track') return `#/track/${r.t}`;
  if (r.view === 'plugin') return `#/track/${r.t}/plugin/${r.p}`;
  return '#/mixer';
}

export const ui = $state({
  route: parseHash() as Route,
  showMix: loadShowMix(),
  size: 'desktop' as 'phone' | 'tablet' | 'desktop',
  short: false,
  sheet: null as Sheet | null,
  pinMode: false,
  filter: '',
  confirm: '' as string,
});

export function go(r: Route) {
  ui.route = r;
  ui.pinMode = false;
  ui.filter = '';
  ui.confirm = '';
  ui.sheet = null;
  const h = routeHash(r);
  if (location.hash !== h) history.replaceState(null, '', h);
}
window.addEventListener('hashchange', () => { ui.route = parseHash(); });

export function toggleMix() {
  ui.showMix = !ui.showMix;
  try { localStorage.setItem('sushi.showMix', ui.showMix ? '1' : '0'); } catch { /* storage unavailable */ }
}

export const trackKey = (s: State, t: Track): string =>
  t === s.pre ? 'pre' : t === s.master ? 'master' : String(t.id);

export function findTrack(s: State | null, key: string): Track | null {
  if (!s) return null;
  if (key === 'pre') return s.pre;
  if (key === 'master') return s.master;
  const id = +key;
  return s.tracks.find(t => t.id === id) ?? (s.pre?.id === id ? s.pre : null) ?? (s.master?.id === id ? s.master : null);
}

export function navOrder(s: State): string[] {
  const ids = s.tracks.map(t => String(t.id));
  if (!s.ui.advanced) return ids;
  return [...(s.pre ? ['pre'] : []), ...ids, ...(s.master ? ['master'] : [])];
}

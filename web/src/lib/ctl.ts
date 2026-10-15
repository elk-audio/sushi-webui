import { paramText } from './util';
import type { Ctl, Param, Send, Track } from './types';
import { setParam, setSend } from './net/socket';
import { pkey } from './state.svelte';
import { dbFromPos, fmtDb, fmtPan, posFromDb } from './util';

const dbOf = (p: Param) => p.min + p.n * (p.max - p.min);
const normOf = (p: Param, db: number) => (p.max === p.min ? 0 : (db - p.min) / (p.max - p.min));

export function gainCtl(t: Track): Ctl | null {
  const p = t.gain;
  if (!p) return null;
  return {
    key: pkey(t.id, p.id), label: 'Gain',
    get: () => posFromDb(dbOf(p)),
    set: v => {
      let db = dbFromPos(v);
      if (Math.abs(db) < 0.25) db = 0;           // unity detent
      db = Math.min(p.max, Math.max(p.min, db));
      p.n = normOf(p, db);
      setParam(t.id, p.id, p.n);
    },
    fmt: () => fmtDb(dbOf(p)),
    def: posFromDb(0),
  };
}

export function panCtl(t: Track): Ctl | null {
  const p = t.pan;
  if (!p) return null;
  const dom = () => p.min + p.n * (p.max - p.min);
  return {
    key: pkey(t.id, p.id), label: 'Pan', bipolar: true,
    get: () => p.n,
    set: v => {
      let n = v;
      const centre = normOf(p, 0);
      if (Math.abs(n - centre) < 0.01) n = centre;  // centre detent
      p.n = n;
      setParam(t.id, p.id, n);
    },
    fmt: () => fmtPan(dom()),
    def: normOf(p, 0),
  };
}

export function sendCtl(t: Track, s: Send): Ctl {
  return {
    key: `send:${t.id}:${s.bus}`, label: '→ ' + s.busName,
    get: () => posFromDb(s.db),
    set: v => { s.db = Math.max(-120, Math.min(24, dbFromPos(v))); setSend(t.id, s.bus, s.db); },
    fmt: () => fmtDb(s.db),
    def: posFromDb(-12),
  };
}

export function paramCtl(procId: number, p: Param): Ctl {
  const steps = p.type === 'bool' ? 1 : p.type === 'int' ? Math.max(1, Math.round(p.max - p.min)) : undefined;
  return {
    key: pkey(procId, p.id), label: p.label || p.name, steps,
    bipolar: p.type === 'float' && p.min < 0 && p.max > 0 && Math.abs(p.min + p.max) < 1e-6,
    get: () => p.n,
    set: v => {
      const n = steps ? Math.round(v * steps) / steps : v;
      if (n === p.n) return;
      p.n = n;
      setParam(procId, p.id, n);
    },
    fmt: () => paramText(p),
  };
}

import { applyParams, setState, store, toast } from '../state.svelte';
import { decodeMeters, silenceAll } from './meters';
import type { State } from '../types';

let ws: WebSocket | null = null;
let backoff = 500;
let nextId = 1;
const pending = new Map<number, { resolve: (v: unknown) => void; reject: (e: Error) => void }>();

// Outgoing continuous values (param sets, send levels): latest value per key, flushed once per frame.
const outbox = new Map<string, unknown>();
let flushScheduled = false;

export function connect() {
  const url = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws`;
  const sock = new WebSocket(url);
  sock.binaryType = 'arraybuffer';
  ws = sock;
  sock.onopen = () => { store.sockOpen = true; backoff = 500; };
  sock.onclose = () => {
    if (ws !== sock) return;
    store.sockOpen = false;
    ws = null;
    silenceAll();
    for (const p of pending.values()) p.reject(new Error('Connection lost'));
    pending.clear();
    store.busy = 0;
    setTimeout(connect, backoff);
    backoff = Math.min(5000, backoff * 2);
  };
  sock.onmessage = ev => {
    if (typeof ev.data !== 'string') { decodeMeters(ev.data as ArrayBuffer); return; }
    let m: any;
    try { m = JSON.parse(ev.data); } catch { return; }
    switch (m.t) {
      case 'hello': store.clientId = m.client; break;
      case 'state': setState(m.state as State, m.rev); break;
      case 'params': applyParams(m.u); break;
      case 'transport':
        if (store.state) {
          if (m.tempo !== undefined) store.state.transport.tempo = m.tempo;
          if (m.playing !== undefined) store.state.transport.playing = m.playing;
        }
        break;
      case 'cpu': if (store.state) store.state.cpu = m.v; break;
      case 'conn':
        if (store.state) store.state.sushi.connected = !!m.sushi;
        if (!m.sushi) silenceAll();
        break;
      case 'result': {
        const p = pending.get(m.id);
        if (!p) break;
        pending.delete(m.id);
        store.busy = Math.max(0, store.busy - 1);
        if (m.ok) p.resolve(m.data);
        else p.reject(new Error(m.error || 'Command failed'));
        break;
      }
    }
  };
}

/** Run a command; errors are shown as a toast and re-thrown. */
export function cmd(op: string, args: Record<string, unknown> = {}): Promise<unknown> {
  if (!ws || ws.readyState !== WebSocket.OPEN) {
    toast('Not connected to sushi-webui');
    return Promise.reject(new Error('Not connected'));
  }
  const id = nextId++;
  store.busy++;
  ws.send(JSON.stringify({ t: 'cmd', id, op, args }));
  return new Promise((resolve, reject) => {
    pending.set(id, {
      resolve,
      reject: e => { toast(e.message); reject(e); },
    });
  });
}

/** Fire a command and swallow the (already toasted) error. */
export function act(op: string, args: Record<string, unknown> = {}) {
  cmd(op, args).catch(() => {});
}

function schedule() {
  if (flushScheduled) return;
  flushScheduled = true;
  requestAnimationFrame(() => {
    flushScheduled = false;
    if (!ws || ws.readyState !== WebSocket.OPEN) { outbox.clear(); return; }
    for (const msg of outbox.values()) ws.send(JSON.stringify(msg));
    outbox.clear();
  });
}

export function setParam(p: number, k: number, n: number) {
  outbox.set(`p${p}:${k}`, { t: 'set', p, k, n });
  schedule();
}
export function setSend(track: number, bus: number, db: number) {
  outbox.set(`s${track}:${bus}`, { t: 'send', track, bus, db });
  schedule();
}

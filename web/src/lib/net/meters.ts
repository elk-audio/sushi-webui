// Binary meter frames (PROTOCOL.md): u8 type=1 | u16 count | count × (u16 slot | u8 L | u8 R | u8 flags)
export const MAX_SLOTS = 256;
export const levels = new Float32Array(MAX_SLOTS * 2).fill(-120); // latest target dB per slot/channel
export const clips = new Uint8Array(MAX_SLOTS * 2);                // latched clip flags
export let lastFrame = 0;

const byteToDb = (b: number) => (b === 0 ? -120 : -60 + (b * 66) / 255);

export function decodeMeters(buf: ArrayBuffer) {
  const v = new DataView(buf);
  if (v.byteLength < 3 || v.getUint8(0) !== 1) return;
  const n = v.getUint16(1, true);
  let o = 3;
  for (let i = 0; i < n && o + 5 <= v.byteLength; i++, o += 5) {
    const slot = v.getUint16(o, true);
    if (slot >= MAX_SLOTS) continue;
    levels[slot * 2] = byteToDb(v.getUint8(o + 2));
    levels[slot * 2 + 1] = byteToDb(v.getUint8(o + 3));
    const f = v.getUint8(o + 4);
    if (f & 1) clips[slot * 2] = 1;
    if (f & 2) clips[slot * 2 + 1] = 1;
  }
  lastFrame = performance.now();
}

export function clearClip(slot: number) {
  if (slot < 0 || slot >= MAX_SLOTS) return;
  clips[slot * 2] = 0;
  clips[slot * 2 + 1] = 0;
}

export function silenceAll() { levels.fill(-120); }

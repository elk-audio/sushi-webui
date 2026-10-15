export const clamp = (v: number, a = 0, b = 1) => Math.min(b, Math.max(a, v));

// Fader law (UX_DESIGN §7): +24..0 dB top 25 %, 0..−20 next 45 %, −20..−60 next 25 %, −60..−120 last 5 %.
export function posFromDb(db: number): number {
  if (db >= 0) return 0.75 + 0.25 * Math.min(db, 24) / 24;
  if (db >= -20) return 0.30 + 0.45 * (db + 20) / 20;
  if (db >= -60) return 0.05 + 0.25 * (db + 60) / 40;
  return 0.05 * Math.max(0, db + 120) / 60;
}
export function dbFromPos(p: number): number {
  if (p >= 0.75) return (p - 0.75) / 0.25 * 24;
  if (p >= 0.30) return (p - 0.30) / 0.45 * 20 - 20;
  if (p >= 0.05) return (p - 0.05) / 0.25 * 40 - 60;
  return p / 0.05 * 60 - 120;
}
export const fmtDb = (db: number) =>
  db <= -119.5 ? '−∞ dB' : (db > 0.04 ? '+' : db < -0.04 ? '−' : '') + Math.abs(db).toFixed(1) + ' dB';
export const fmtPan = (p: number) =>
  Math.abs(p) < 0.005 ? 'C' : (p < 0 ? 'L ' : 'R ') + Math.round(Math.abs(p) * 100);

export const PALETTE = ['#e0a33a', '#5aa0d8', '#c9719a', '#58b38b', '#9b86d9', '#d9734e', '#4fb3b8', '#b8b04f'];

export function chanLabel(prefix: string, first: number, count: number): string {
  return count > 1 ? `${prefix} ${first + 1}-${first + count}` : `${prefix} ${first + 1}`;
}

/** Sushi's formatted_value for internal plugins has no unit ("4000.00"); append the parameter unit when missing. */
export function paramText(p: { txt: string; unit: string }): string {
  const u = p.unit.trim();
  if (!u || !p.txt || p.txt.toLowerCase().includes(u.toLowerCase())) return p.txt;
  return `${p.txt} ${u}`;
}

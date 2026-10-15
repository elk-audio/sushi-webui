import { levels, clips } from '../net/meters';

// One requestAnimationFrame loop for every meter on the page (ARCHITECTURE §6.3).
interface MeterEl {
  cv: HTMLCanvasElement; ctx: CanvasRenderingContext2D;
  slot: number; vert: boolean; visible: boolean;
  w: number; h: number; grad: CanvasGradient | null;
}
const meters = new Set<MeterEl>();
const disp = new Float32Array(512).fill(-120);   // displayed dB per slot/channel
const peak = new Float32Array(512).fill(-120);
const peakT = new Float64Array(512);

const io = typeof IntersectionObserver !== 'undefined'
  ? new IntersectionObserver(entries => {
      for (const e of entries) for (const m of meters) if (m.cv === e.target) m.visible = e.isIntersecting;
    })
  : null;

const frac = (db: number) => Math.min(1, Math.max(0, (db + 60) / 66));

function resize(m: MeterEl) {
  const dpr = window.devicePixelRatio || 1;
  const w = Math.max(1, Math.round(m.cv.clientWidth * dpr));
  const h = Math.max(1, Math.round(m.cv.clientHeight * dpr));
  if (w !== m.w || h !== m.h) {
    m.cv.width = m.w = w; m.cv.height = m.h = h;
    const g = m.vert ? m.ctx.createLinearGradient(0, h, 0, 0) : m.ctx.createLinearGradient(0, 0, w, 0);
    const y = (db: number) => (db + 60) / 66;
    g.addColorStop(0, '#2f9e6a'); g.addColorStop(y(-12), '#45c486');
    g.addColorStop(y(-12) + 0.001, '#e8bf45'); g.addColorStop(y(-3), '#e8bf45');
    g.addColorStop(y(-3) + 0.001, '#ec4d4f'); g.addColorStop(1, '#ec4d4f');
    m.grad = g;
  }
}

export function registerMeter(cv: HTMLCanvasElement, slot: number, vert: boolean) {
  const ctx = cv.getContext('2d');
  if (!ctx) return { update() {}, destroy() {} };
  const m: MeterEl = { cv, ctx, slot, vert, visible: true, w: 0, h: 0, grad: null };
  meters.add(m);
  io?.observe(cv);
  return {
    update(slot: number) { m.slot = slot; },
    destroy() { meters.delete(m); io?.unobserve(cv); },
  };
}

let last = performance.now();
let frameNo = 0;
function frame(now: number) {
  const dt = Math.min(0.1, (now - last) / 1000);
  last = now;
  frameNo++;
  for (let i = 0; i < 512; i++) {
    const t = levels[i];
    if (t > disp[i]) disp[i] += (t - disp[i]) * 0.7;
    else disp[i] = Math.max(t, disp[i] - 26 * dt);
    if (disp[i] >= peak[i]) { peak[i] = disp[i]; peakT[i] = now; }
    else if (now - peakT[i] > 1500) peak[i] = Math.max(disp[i], peak[i] - 20 * dt);
  }
  for (const m of meters) {
    if (!m.visible || m.slot < 0) continue;
    if (frameNo % 30 === 0 || !m.grad) resize(m);
    draw(m);
  }
  requestAnimationFrame(frame);
}
requestAnimationFrame(frame);

function draw(m: MeterEl) {
  const { ctx, w, h, grad } = m;
  if (!grad) return;
  const s = m.slot * 2;
  const clip = clips[s] || clips[s + 1];
  ctx.clearRect(0, 0, w, h);
  if (m.vert) {
    const led = Math.round(h * 0.05) + 2, gap = Math.max(1, Math.round(w * 0.12)), bw = (w - gap) / 2, H = h - led - 2;
    ctx.fillStyle = clip ? '#ec4d4f' : '#2f3642'; ctx.fillRect(0, 0, w, led);
    for (let c = 0; c < 2; c++) {
      const x = c * (bw + gap);
      ctx.globalAlpha = 0.14; ctx.fillStyle = grad; ctx.fillRect(x, led + 2, bw, H); ctx.globalAlpha = 1;
      const lv = frac(disp[s + c]) * H; ctx.fillRect(x, h - lv, bw, lv);
      const pk = frac(peak[s + c]);
      if (pk > 0.01) { ctx.fillStyle = '#e7e9ee'; ctx.fillRect(x, h - pk * H, bw, Math.max(1, h * 0.006)); }
    }
  } else {
    const led = Math.round(w * 0.025) + 2, gap = Math.max(1, Math.round(h * 0.15)), bh = (h - gap) / 2, W = w - led - 2;
    for (let c = 0; c < 2; c++) {
      const y = c * (bh + gap);
      ctx.globalAlpha = 0.12; ctx.fillStyle = grad; ctx.fillRect(0, y, W, bh); ctx.globalAlpha = 1;
      ctx.fillRect(0, y, frac(disp[s + c]) * W, bh);
      const pk = frac(peak[s + c]);
      if (pk > 0.01) { ctx.fillStyle = '#e7e9ee'; ctx.fillRect(pk * W, y, Math.max(1, w * 0.004), bh); }
    }
    ctx.fillStyle = clip ? '#ec4d4f' : '#2f3642'; ctx.fillRect(w - led, 0, led, h);
  }
}

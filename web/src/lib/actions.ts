import type { State, Track } from './types';
import { act, cmd } from './net/socket';
import { ui } from './ui.svelte';

// Optimistic toggles: flip locally, the next state from the backend confirms or corrects.
export function toggleMute(t: Track) { t.mute = !t.mute; act('setMute', { track: t.id, on: t.mute }); }
export function toggleSolo(t: Track) { t.solo = !t.solo; act('setSolo', { track: t.id, on: t.solo }); }

// Removing a plugin is asked for first (ui.confirm = confirmKey(proc)); the backend confirms by sending the new state.
export const confirmKey = (proc: number) => `delplug:${proc}`;
export async function removePlugin(proc: number) {
  ui.confirm = '';
  try { await cmd('removePlugin', { proc }); } catch { /* toasted */ }
}

// Deleting a track is asked for first (ui.confirm = trackConfirmKey(track)). The route falls back to the mixer by itself.
export const trackConfirmKey = (t: Track) => `deltrack:${t.id}`;
export function trackConfirmText(s: State, t: Track) {
  const n = t.chain.length;
  const fed = t.kind === 'bus' ? [...s.tracks, ...(s.pre ? [s.pre] : [])].filter(o => o.sends.some(x => x.bus === t.id)).length : 0;
  const what = [n ? `its ${n} plugin${n > 1 ? 's' : ''}` : '', fed ? `the ${fed} send${fed > 1 ? 's' : ''} into it` : ''].filter(Boolean).join(' and ');
  return `Delete ${t.name}${what ? ' and ' + what : ''}?`;
}
export async function removeTrack(t: Track) {
  ui.confirm = '';
  try { await cmd('removeTrack', { track: t.id }); } catch { /* toasted */ }
}

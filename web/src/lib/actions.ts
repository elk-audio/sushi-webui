import type { Track } from './types';
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

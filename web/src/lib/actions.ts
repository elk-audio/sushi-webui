import type { Track } from './types';
import { act } from './net/socket';

// Optimistic toggles: flip locally, the next state from the backend confirms or corrects.
export function toggleMute(t: Track) { t.mute = !t.mute; act('setMute', { track: t.id, on: t.mute }); }
export function toggleSolo(t: Track) { t.solo = !t.solo; act('setSolo', { track: t.id, on: t.solo }); }

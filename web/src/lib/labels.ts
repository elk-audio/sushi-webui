import type { Role, State, Track } from './types';

export const ROLE_LABEL: Record<Role, string> = { midi_fx: 'MIDI effects', instrument: 'Instrument', audio_fx: 'Audio effects' };
export const ROLE_ADD: Record<Role, string> = { midi_fx: 'MIDI effect', instrument: 'instrument', audio_fx: 'effect' };

export const sendersOf = (s: State, bus: Track) => s.tracks.filter(t => t.sends.some(sd => sd.bus === bus.id));
export const isFixed = (t: Track) => t.kind === 'pre' || t.kind === 'post';
export const sectionsFor = (t: Track): Role[] => (t.kind === 'midi' ? ['midi_fx', 'instrument', 'audio_fx'] : ['audio_fx']);

export function kindHtml(s: State, t: Track): string {
  const e = esc;
  switch (t.kind) {
    case 'midi': return `<b>MIDI</b> · ${e(t.input.replace(/^MIDI /, ''))}`;
    case 'audio': return `<b>AUDIO</b> · ${e(t.input)}`;
    case 'bus': return `<b>BUS</b> · ${sendersOf(s, t).length} sends`;
    case 'pre': return `<b>INPUT</b> · ${e(t.input)}`;
    case 'post': return s.ui.advanced ? `<b>OUTPUT</b> · ${e(t.output)}` : e(t.output);
  }
}

export function routeIn(s: State, t: Track): [string, string] {
  switch (t.kind) {
    case 'midi': return ['MIDI IN', t.input];
    case 'audio': return ['AUDIO IN', t.input];
    case 'bus': return ['FROM SENDS', sendersOf(s, t).map(x => x.name).join(', ') || 'none yet'];
    case 'pre': return ['ENGINE IN', t.input];
    case 'post': return ['ALL TRACKS', 'summed'];
  }
}

export function esc(s: string): string {
  return s.replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]!));
}

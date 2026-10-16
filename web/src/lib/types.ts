// Mirrors docs/PROTOCOL.md (v1).
export type Kind = 'audio' | 'midi' | 'bus' | 'pre' | 'post';
export type Role = 'audio_fx' | 'midi_fx' | 'instrument';

export interface Param {
  id: number; name: string; label: string; unit: string;
  type: 'float' | 'int' | 'bool'; automatable: boolean;
  min: number; max: number;
  n: number; txt: string;
}
export interface Prop { id: number; name: string; label: string; value: string }
export interface Processor {
  id: number; name: string; label: string;
  uid: string; type: 'internal' | 'vst2' | 'vst3' | 'lv2' | 'unknown'; role: Role;
  bypass: boolean; program: number; programs: string[];
  params: Param[]; props: Prop[]; macros: number[];
}
export interface Send { bus: number; busName: string; db: number; post: boolean }
export interface Track {
  id: number; name: string; kind: Kind; color: string; channels: number;
  gain: Param | null; pan: Param | null;
  mute: boolean; solo: boolean;
  input: string; output: string;
  chain: Processor[]; sends: Send[];
  meter: number;
}
export interface CatalogEntry { id: string; name: string; vendor: string; category: string; uid: string; path: string; type: 'internal' | 'vst2' | 'vst3' | 'lv2'; role: Role }
export interface State {
  sushi: { connected: boolean; version: string; inputs: number; outputs: number; midiInputs: number; samplerate: number };
  project: { name: string; dirty: boolean; list: string[] };
  ui: { advanced: boolean };
  transport: { tempo: number; playing: boolean };
  cpu: number;
  tracks: Track[];
  pre: Track | null;
  master: Track | null;
  catalog: CatalogEntry[];
}

/** A control a widget can drive: position is always 0..1. */
export interface Ctl {
  key: string;                 // unique id, used for drag/echo bookkeeping
  label: string;
  get(): number;
  set(v: number): void;
  fmt(): string;
  def?: number;                // reset position, if any
  bipolar?: boolean;
  steps?: number;              // quantization steps (int/bool params)
}

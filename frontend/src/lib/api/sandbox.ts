// An instance's sandbox: Go's sandbox status and rules shaped for the Sandbox page.
// The browser preview serves the ISetC board's rows and follows its presets.
import { bridged } from '../bridge';
import { revs } from '../revs.svelte';
import * as SandboxGo from '../wailsjs/go/api/Sandbox';

export type SandboxLevel = 'rw' | 'ro' | 'no' | 'on' | 'remove';
export type SandboxPreset = 'standard' | 'strict' | 'custom';

/** Whether games can be sandboxed here, and with what ("bubblewrap" 0.11.1). */
export interface SandboxStatus { available: boolean; name: string; version: string; reason: string }
/** One row of "What the game can reach": a rule, or one of the player's folders. */
export interface SandboxRow { id: string; folder: boolean; path: string; level: SandboxLevel; choices: SandboxLevel[] }
export interface SandboxView { on: boolean; preset: SandboxPreset; rows: SandboxRow[] }

const STANDARD: Record<string, SandboxLevel> = { instance: 'rw', mods: 'ro', libraries: 'ro', java: 'ro', synced: 'rw', internet: 'on', discord: 'on', graphics: 'on', home: 'no' };
const STRICT: Record<string, SandboxLevel> = { ...STANDARD, synced: 'ro', internet: 'no', discord: 'no' };
const CHOICES: Record<string, SandboxLevel[]> = {
  instance: ['rw'], mods: ['ro'], libraries: ['ro'], java: ['ro'], synced: ['rw', 'ro', 'no'], internet: ['on', 'no'], discord: ['on', 'no'], graphics: ['on'], home: ['no', 'ro'],
};
const PATHS: Record<string, string> = {
  instance: 'instances/fabulously-optimized/', mods: 'instances/fabulously-optimized/game/mods/', libraries: 'libraries/, assets/', java: 'java/temurin-21.0.5/', synced: 'sync/options, sync/servers',
  internet: '', discord: '', graphics: '', home: '',
};

/** The preview's one sandbox, following the same preset rules as Go. */
const mock = {
  view: { on: true, preset: 'standard' as SandboxPreset, levels: { ...STANDARD }, folders: [] as SandboxRow[] },
  get(): SandboxView {
    const v = mock.view;
    const rows = Object.keys(STANDARD).map((id): SandboxRow => ({ id, folder: false, path: PATHS[id]!, level: v.levels[id]!, choices: CHOICES[id]! }));
    return { on: v.on, preset: v.preset, rows: [...rows, ...v.folders] };
  },
  setOn(on: boolean) { mock.view.on = on; return mock.get(); },
  setPreset(p: SandboxPreset) {
    mock.view.preset = p;
    if (p !== 'custom') mock.view.levels = { ...(p === 'strict' ? STRICT : STANDARD) };
    return mock.get();
  },
  setLevel(rule: string, level: SandboxLevel) {
    if (mock.view.levels[rule] !== level) { mock.view.levels[rule] = level; mock.view.preset = 'custom'; }
    return mock.get();
  },
  addFolder() {
    mock.view.folders.push({ id: `f${mock.view.folders.length}`, folder: true, path: '~/Games/maps', level: 'ro', choices: ['rw', 'ro', 'remove'] });
    return mock.get();
  },
  setFolder(id: string, level: SandboxLevel) {
    mock.view.folders = level === 'remove' ? mock.view.folders.filter((f) => f.id !== id) : mock.view.folders.map((f) => (f.id === id ? { ...f, level } : f));
    return mock.get();
  },
};

export const sandboxApi = {
  status: bridged(SandboxGo.Status, (): SandboxStatus => ({ available: true, name: 'bubblewrap', version: '0.9.0', reason: '' })),
  /** Reloads when the instance or what it shares changes (the Synced row's paths). */
  view: (id: string) => revs.load('sync', () => revs.load('instances', bridged(() => SandboxGo.View(id), () => mock.get()))),
  setOn: bridged(SandboxGo.SetOn, (_id: string, on: boolean) => mock.setOn(on)),
  setPreset: bridged(SandboxGo.SetPreset, (_id: string, p: SandboxPreset) => mock.setPreset(p)),
  setLevel: bridged(SandboxGo.SetLevel, (_id: string, rule: string, l: SandboxLevel) => mock.setLevel(rule, l)),
  /** Opens a native folder picker; resolves unchanged when cancelled. */
  addFolder: bridged(SandboxGo.AddFolder, (_id: string) => mock.addFolder()),
  setFolder: bridged(SandboxGo.SetFolder, (_id: string, folder: string, l: SandboxLevel) => mock.setFolder(folder, l)),
};

// Browser-preview restore points: an open session, one the game crashed
// after, one made by hand and one about to expire, and a rollback list that
// shows every kind of row. Edits stay in memory.
import type { RestoreChange, RestorePoint, RollbackResult, SnapshotDiff, SnapshotRow } from '../types';

const HOUR = 3_600_000;
const DAY = 24 * HOUR;
const WEEK = 7 * DAY;
const now = Date.now();
const change = (id: string, ago: number, title: string, reason = 'update', undoable = false): RestoreChange => ({ id, at: now - ago, title, reason, undoable });
const point = (id: string, ago: number, changes: RestoreChange[], extra: Partial<RestorePoint> = {}): RestorePoint => {
  const p: RestorePoint = { id, at: now - ago, keep: false, manual: false, changes, ...extra };
  return p.sealed && !p.keep && !extra.expires ? { ...p, expires: p.sealed + WEEK } : p;
};

let list: RestorePoint[] = [
  point('1800000000000-a1b2', 2 * HOUR, [change('1800000000000-a1b2', 2 * HOUR, 'updating 12 mods', 'update', true), change('1800000000500-a1b3', HOUR, 'installing Distant Horizons', 'install', true)]),
  point('1799900000000-c3d4', 3 * DAY, [change('1799900000000-c3d4', 3 * DAY, 'moving to Minecraft 1.21.4', 'version'), change('1799900000300-c3d5', 3 * DAY, 'updating Fabric Loader to 0.16.10', 'loader'), change('1799900000600-c3d6', 3 * DAY, 'updating 4 mods')], { sealed: now - 3 * DAY + HOUR }),
  point('1799800000000-e5f6', 5 * DAY, [], { manual: true, keep: true, sealed: now - 5 * DAY }),
  point('1799700000000-a7b8', 7 * DAY, [change('1799700000000-a7b8', 7 * DAY, 'updating Iris Shaders')], { sealed: now - 6 * DAY, expires: now + DAY }),
];

const rows: SnapshotRow[] = [
  { key: 'version', kind: 'version', change: 'set', name: 'Minecraft and loader', from: '1.21.4, Fabric 0.16.10', to: '1.21.1, Fabric 0.16.9', default: true },
  { key: 'item:0a1b2c3d4e5f6071', kind: 'mods', change: 'changed', name: 'Sodium', from: '0.6.0', to: '0.5.11', default: true },
  { key: 'item:1a1b2c3d4e5f6071', kind: 'mods', change: 'changed', name: 'Fabric API', from: '0.110.0', to: '0.102.0', default: true },
  { key: 'item:2a1b2c3d4e5f6071', kind: 'mods', change: 'changed', name: 'Iris Shaders', from: '1.8.1', to: '1.7.6', detail: 'and turned on', default: true },
  { key: 'item:3a1b2c3d4e5f6071', kind: 'mods', change: 'restore', name: 'Indium', to: '1.0.35', default: true },
  { key: 'item:4a1b2c3d4e5f6071', kind: 'mods', change: 'remove', name: 'Distant Horizons', from: '2.3.0', default: true },
  { key: 'item:5a1b2c3d4e5f6071', kind: 'mods', change: 'disable', name: 'Mod Menu', from: '11.0.3', to: '11.0.3', default: true },
  { key: 'item:6a1b2c3d4e5f6071', kind: 'mods', change: 'changed', name: 'Cloth Config', from: '15.0.140', to: '14.0.126', default: false, unavailable: 'Its old file is gone, so it can’t be put back' },
  { key: 'item:7a1b2c3d4e5f6071', kind: 'resourcepacks', change: 'changed', name: 'Fresh Animations', from: '1.9.2', to: '1.9.1', default: true },
  { key: 'packs', kind: 'packs', change: 'set', name: 'Resource packs in use', from: '2 packs', to: '1 pack', detail: 'Fresh Animations', default: true },
  { key: 'files', kind: 'files', change: 'set', name: 'Config files', detail: '4 files come back, 1 new file goes', default: true },
];

const find = (id: string) => list.find((p) => p.id === id) ?? list[0]!;

export const mockSnapshots = {
  list: (_id: string) => list,
  diff: (_id: string, id: string): SnapshotDiff => ({ point: find(id), rows, shared: ['saves'], undoable: true }),
  create: (_id: string): RestorePoint => {
    const p = point(`${Date.now()}-0000`, 0, [], { manual: true, sealed: Date.now() });
    list = [p, ...list];
    return p;
  },
  setKeep: (_id: string, id: string, keep: boolean): RestorePoint => {
    list = list.map((p) => {
      if (p.id !== id) return p;
      const q = { ...p, keep };
      if (keep) delete q.expires;
      else if (q.sealed) q.expires = q.sealed + WEEK;
      return q;
    });
    return find(id);
  },
  remove: (_id: string, id: string) => {
    list = list.filter((p) => p.id !== id);
  },
  rollback: (_id: string, _point: string, keys: string[]): RollbackResult => ({ restored: keys.length, snapshot: '1800000000001-0000' }),
  undo: (_id: string, _change: string): RollbackResult => ({ restored: 3, snapshot: '1800000000001-0000' }),
  suspect: (_id: string): RestorePoint | null => list[1] ?? null,
};

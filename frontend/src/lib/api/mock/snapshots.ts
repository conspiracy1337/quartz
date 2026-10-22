// Browser-preview snapshots: one per kind of change, and a rollback list that
// shows every kind of row. Edits stay in memory.
import type { RollbackResult, SnapshotDiff, SnapshotRow, SnapshotSummary } from '../types';

const HOUR = 3_600_000;
const WEEK = 7 * 24 * HOUR;
const now = Date.now();
const snap = (id: string, ago: number, title: string, reason: string, keep = false): SnapshotSummary =>
  ({ id, at: now - ago, title, reason, keep, size: 184_000, ...(keep ? {} : { expires: now - ago + WEEK }) });

let list: SnapshotSummary[] = [
  snap('1800000000000-a1b2', 2 * HOUR, 'updating 12 mods', 'update'),
  snap('1799900000000-c3d4', 30 * HOUR, 'moving to Minecraft 1.21.4', 'version', true),
  snap('1799800000000-e5f6', 4 * 24 * HOUR, '', 'manual'),
];

const rows: SnapshotRow[] = [
  { key: 'version', kind: 'version', change: 'set', name: 'Minecraft and loader', from: '1.21.4, Fabric 0.16.10', to: '1.21.1, Fabric 0.16.9', default: true },
  { key: 'item:0a1b2c3d4e5f6071', kind: 'mods', change: 'changed', name: 'Sodium', from: '0.6.0', to: '0.5.11', default: true },
  { key: 'item:1a1b2c3d4e5f6071', kind: 'mods', change: 'changed', name: 'Fabric API', from: '0.110.0', to: '0.102.0', default: true },
  { key: 'item:2a1b2c3d4e5f6071', kind: 'mods', change: 'changed', name: 'Iris Shaders', from: '1.8.1', to: '1.7.6', detail: 'and turned on', default: true },
  { key: 'item:3a1b2c3d4e5f6071', kind: 'mods', change: 'restore', name: 'Indium', to: '1.0.35', default: true },
  { key: 'item:4a1b2c3d4e5f6071', kind: 'mods', change: 'remove', name: 'Cloth Config', from: '15.0.140', default: true },
  { key: 'item:5a1b2c3d4e5f6071', kind: 'mods', change: 'disable', name: 'Mod Menu', from: '11.0.3', to: '11.0.3', default: false },
  { key: 'item:6a1b2c3d4e5f6071', kind: 'mods', change: 'changed', name: 'Entity Culling', from: '1.7.2', to: '1.6.4', default: false, unavailable: 'Its old file is gone' },
  { key: 'item:7a1b2c3d4e5f6071', kind: 'resourcepacks', change: 'changed', name: 'Fresh Animations', from: '1.9.2', to: '1.9.1', default: true },
  { key: 'packs', kind: 'packs', change: 'set', name: 'Resource packs in use', from: '2 packs', to: '1 pack', detail: 'Fresh Animations', default: false },
  { key: 'files', kind: 'files', change: 'set', name: 'Config files', detail: '4 files come back, 1 new file goes', default: false },
];

const find = (id: string) => list.find((s) => s.id === id) ?? list[0]!;

export const mockSnapshots = {
  list: (_id: string) => list,
  diff: (_id: string, id: string): SnapshotDiff => ({ snapshot: find(id), rows, shared: ['saves'], undoable: true }),
  create: (_id: string): SnapshotSummary => {
    const s = snap(`${Date.now()}-0000`, 0, '', 'manual');
    list = [s, ...list];
    return s;
  },
  setKeep: (_id: string, id: string, keep: boolean): SnapshotSummary => {
    list = list.map((s) => (s.id === id ? snap(s.id, now - s.at, s.title, s.reason, keep) : s));
    return find(id);
  },
  remove: (_id: string, id: string) => {
    list = list.filter((s) => s.id !== id);
  },
  rollback: (_id: string, _snap: string, keys: string[]): RollbackResult => ({ restored: keys.length, snapshot: '1800000000001-0000' }),
  undo: (_id: string, _snap: string): RollbackResult => ({ restored: rows.filter((r) => r.default).length, snapshot: '1800000000001-0000' }),
  suspect: (_id: string): SnapshotSummary | null => list[0] ?? null,
};

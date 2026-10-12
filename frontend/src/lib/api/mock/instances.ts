import type { ActivityEvent, Instance, PendingUpdate } from '../types';
import { daysAgo, date } from './time';

type Seed = [id: string, name: string, initial: string, tint: string, mc: string, loader: Instance['loader'], kind: Instance['kind'], lastDays: number | undefined, hours: number];

const seeds: Seed[] = [
  ['fo', 'Fabulously Optimized', 'F', '#3ecf8e', '1.21.4', { kind: 'fabric', version: '0.16.10' }, 'modpack', 1, 41],
  ['cab', 'Create: Above and Beyond', 'C', '#e2a03f', '1.16.5', { kind: 'forge', version: '36.2.39' }, 'modpack', 2, 118],
  ['van', 'Vanilla 1.21.4', 'V', '#6b8afd', '1.21.4', undefined, 'vanilla', 4, 9],
  ['prom', 'Prominence II: Hasturian Era', 'P', '#b48cf2', '1.20.1', { kind: 'fabric', version: '0.16.9' }, 'modpack', 7, 23],
  ['snap', 'Snapshot 25w14a', 'S', '#c76bd8', '25w14a', undefined, 'vanilla', 14, 2],
  ['hc', 'Hardcore Season 3', 'H', '#9aa3ad', '1.21.1', { kind: 'neoforge', version: '21.1.77' }, 'modded', 21, 64],
  ['sky', 'Skyblock Classic', 'S', '#5bc0be', '1.21.1', { kind: 'fabric', version: '0.16.9' }, 'modded', 30, 12],
  ['bm', 'Better Minecraft [FORGE]', 'B', '#e86f6f', '1.20.1', { kind: 'forge', version: '47.3.0' }, 'modpack', 60, 31],
  ['red', 'Redstone Testing', 'R', '#f2d45c', '1.21.4', { kind: 'fabric', version: '0.16.10' }, 'modded', 90, 4],
  ['atm', 'All the Mods 10', 'A', '#7cc7e8', '1.21.1', { kind: 'neoforge', version: '21.1.77' }, 'modpack', 120, 52],
  ['cob', 'Cobblemon Official', 'C', '#f2a25c', '1.21.1', { kind: 'fabric', version: '0.16.9' }, 'modpack', 150, 17],
  ['quilt', 'Quilted Test', 'Q', '#d38cf2', '1.20.4', { kind: 'quilt', version: '0.27.1' }, 'modded', undefined, 0],
];

const withServers = new Set(['fo', 'van', 'sky', 'cob', 'prom']);

export const instances: Instance[] = seeds.map(([id, name, initial, tint, minecraft, loader, kind, lastDays, hours]) => ({
  id, name, minecraft, kind,
  ...(loader ? { loader } : {}),
  art: { background: tint, initial },
  hasServers: withServers.has(id),
  ...(lastDays !== undefined ? { lastPlayed: daysAgo(lastDays, 21, 14) } : {}),
  playtimeMin: hours * 60,
  running: false,
  ...(id === 'fo' ? { icon: 'bolt' as const } : {}),
  sandboxed: true,
  counts: { mods: 8, resourcepacks: 6, shaders: 4, datapacks: 4, worlds: 6, servers: 4 },
  java: '21.0.5, automatic',
  memoryMB: { min: 2048, max: 6144 },
  created: date(2025, 3, 2),
  folder: 'instances/' + name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, ''),
  syncTargets: 4,
  discordStatus: true,
}));

/** Sidebar "Recent" order from the design. */
export const recentIds = ['fo', 'cab', 'van', 'snap'];

export const activity: ActivityEvent[] = [
  { kind: 'session', icon: 'play', tone: 'muted', title: 'Played for 2 h 08 m', detail: 'Northwatch, closed normally', at: daysAgo(1) },
  { kind: 'session', icon: 'camera', tone: 'muted', title: 'Took 3 screenshots', detail: 'Northwatch', at: daysAgo(1) },
  { kind: 'change', icon: 'arrow-up', tone: 'accent', title: 'Updated 4 mods', detail: 'Sodium, Lithium, Fabric API, Mod Menu', at: daysAgo(2) },
  { kind: 'session', icon: 'play', tone: 'muted', title: 'Played for 4 h 41 m', detail: 'Northwatch, closed normally', at: daysAgo(2) },
  { kind: 'session', icon: 'warn', tone: 'danger', title: 'Game crashed after 3 minutes', detail: 'Mixin apply failed in entityculling', at: daysAgo(3) },
  { kind: 'change', icon: 'plus', tone: 'accent', title: 'Added Distant Horizons', detail: 'From Modrinth, with 1 dependency', at: daysAgo(3) },
];

export const pendingUpdates: PendingUpdate[] = [
  { name: 'Iris Shaders', art: { background: '#b48cf2', initial: 'I' }, from: '1.8.1', to: '1.8.3' },
  { name: 'Not Enough Animations', art: { background: '#f2a25c', initial: 'N' }, from: '1.9.2', to: '1.9.4' },
];

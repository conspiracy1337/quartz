import type { ActivityEvent, Art, CreateInstance, Group, Instance, InstanceSettings, Loader, PendingUpdate, TrashEntry } from '../types';
import { daysAgo, date } from './time';

type Seed = [id: string, name: string, initial: string, tint: string, mc: string, loader: Omit<NonNullable<Instance['loader']>, 'id'> | undefined, kind: Instance['kind'], lastDays: number | undefined, hours: number];

/** Go stores Forge builds with their Minecraft version in front ("1.20.1-47.3.0"). */
const buildId = (mc: string, l: { kind: string; version: string }) => (l.kind === 'forge' ? `${mc}-${l.version}` : l.version);

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
  id, name, minecraft, kind, tags: [],
  ...(loader ? { loader: { ...loader, id: buildId(minecraft, loader) } } : {}),
  art: { background: tint, initial },
  hasServers: withServers.has(id),
  ...(lastDays !== undefined ? { lastPlayed: daysAgo(lastDays, 21, 14) } : {}),
  playtimeMin: hours * 60,
  running: false,
  ...(id === 'fo' ? { icon: 'bolt' as const } : {}),
  sandboxed: true,
  counts: { mods: 48, resourcepacks: 6, shaders: 4, datapacks: 4, worlds: 6, servers: 4 },
  java: '21.0.5, automatic',
  javaAuto: 'Java 21.0.5',
  memoryMB: { min: 2048, max: 6144 },
  created: date(2025, 3, 2),
  folder: 'instances/' + name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, ''),
  moved: false,
  notes: '',
  keepLoaderUpdated: true,
  followLatest: false,
  syncTargets: 4,
  discordStatus: true,
  gpu: 'NVIDIA GeForce RTX 3070 Laptop GPU',
  settings: {
    java: { override: true, javaPath: '', memoryMinMb: 2048, memoryMaxMb: 6144, jvmArgs: '-XX:+UseG1GC -XX:MaxGCPauseMillis=50 -XX:+UnlockExperimentalVMOptions' },
    window: { width: 1920, height: 1080, fullscreen: false, onGameStart: 'hide' },
    commands: {
      preLaunch: '$HOME/scripts/backup.sh "$INST_DIR"', wrapper: '', postExit: '',
      env: [{ name: '__GL_THREADED_OPTIMIZATIONS', value: '0' }, { name: 'MESA_GL_VERSION_OVERRIDE', value: '4.6' }],
      systemGlfw: false, systemOpenal: false,
    },
    performance: { dedicatedGpu: true, gameMode: true, mangoHud: false },
  },
}));

export const groups: Group[] = [];
export const trash: TrashEntry[] = [];

const find = (id: string) => instances.find((i) => i.id === id) ?? instances[0]!;
const edit = (id: string, fn: (i: Instance) => void): Instance => {
  const inst = find(id);
  fn(inst);
  return inst;
};

/** Preview stand-in for Go's instance store: changes live until the page reloads. */
export const mockInstances = {
  create: (req: CreateInstance): Instance => {
    const base = instances[2]!;
    const inst: Instance = {
      ...structuredClone(base), id: `new-${instances.length}`, name: req.name, art: req.art, minecraft: req.minecraft, playtimeMin: 0, created: Date.now(), tags: [],
      ...(req.icon ? { icon: req.icon } : {}), ...(req.group ? { group: req.group } : {}),
      kind: req.loader && req.loader !== 'none' ? 'modded' : 'vanilla',
    };
    delete inst.lastPlayed;
    if (req.loader && req.loader !== 'none') inst.loader = { kind: req.loader as Loader, version: req.loaderVersion || 'latest', id: req.loaderVersion || 'latest' };
    else delete inst.loader;
    instances.unshift(inst);
    return inst;
  },
  updateSettings: (id: string, settings: InstanceSettings) => edit(id, (i) => (i.settings = settings)),
  rename: (id: string, name: string) => edit(id, (i) => (i.name = name.trim() || i.name)),
  setArt: (id: string, art: Art) => edit(id, (i) => { i.art = art; delete i.icon; }),
  setIcon: (id: string, icon: string) => edit(id, (i) => { if (icon) i.icon = icon; else delete i.icon; }),
  setNotes: (id: string, notes: string) => edit(id, (i) => (i.notes = notes)),
  setTags: (id: string, tags: string[]) => edit(id, (i) => (i.tags = [...new Set(tags.map((t) => t.trim()).filter(Boolean))].slice(0, 12))),
  setGroup: (id: string, group: string) => edit(id, (i) => {
    if (group) {
      i.group = group;
      if (!groups.some((g) => g.name.toLowerCase() === group.toLowerCase())) groups.push({ name: group });
    } else delete i.group;
  }),
  createGroup: (name: string): Group => {
    const g = { name: name.trim() };
    groups.push(g);
    return g;
  },
  setVersion: (id: string, minecraft: string, kind: string, version: string) => edit(id, (i) => {
    i.minecraft = minecraft;
    if (kind && kind !== 'none') i.loader = { kind: kind as Loader, version: version || i.loader?.version || 'latest', id: version || i.loader?.id || 'latest' };
    else delete i.loader;
    i.kind = i.loader ? 'modded' : 'vanilla';
  }),
  setVersionOptions: (id: string, keep: boolean, follow: boolean) => edit(id, (i) => { i.keepLoaderUpdated = keep; i.followLatest = follow; }),
  trash: (id: string): TrashEntry => {
    const at = instances.findIndex((i) => i.id === id);
    const [inst] = instances.splice(Math.max(at, 0), 1);
    const e: TrashEntry = { key: `${inst!.id}~${Date.now()}`, id: inst!.id, name: inst!.name, art: inst!.art, minecraft: inst!.minecraft, trashedAt: Date.now() };
    trash.unshift(e);
    trashed.set(e.key, inst!);
    return e;
  },
  restore: (key: string): Instance => {
    const inst = trashed.get(key)!;
    trashed.delete(key);
    trash.splice(trash.findIndex((e) => e.key === key), 1);
    instances.unshift(inst);
    return inst;
  },
  purge: (key: string) => {
    trashed.delete(key);
    trash.splice(trash.findIndex((e) => e.key === key), 1);
  },
};
const trashed = new Map<string, Instance>();

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

import type { ExportEntry, ExportPlan, ExportPreview, ImportPreview, InstallPlan, JavaRuntime, KeyChoice, Keybind, ManualView, ModUpdate, ModpackDiffRow, OtherLauncher, PackVersion, SyncEntry, SyncGroups } from '../types';
import { date, daysAgo } from './time';

const S = (id: string, name: string, path: string, users: number, enabled: boolean, available = true): SyncEntry => ({ id, name, path, users, enabled, available });

export const sync: SyncGroups = {
  total: 12,
  files: [S('options', 'Game options', 'options.txt', 8, true), S('servers', 'Server list', 'servers.dat', 12, true), S('history', 'Command history', 'command_history.txt', 5, true), S('hotbar', 'Hotbars', 'hotbar.nbt', 12, true)],
  folders: [S('saves', 'Worlds', 'saves/', 2, false), S('config', 'Configs', 'config/', 0, false), S('screenshots', 'Screenshots', 'screenshots/', 6, true), S('resourcepacks', 'Resource packs', 'resourcepacks/', 9, true), S('shaderpacks', 'Shader packs', 'shaderpacks/', 4, true), S('datapacks', 'Data packs', 'datapacks/', 0, false), S('downloads', 'Server resource packs', 'downloads/', 0, false)],
  mods: [S('xaero', "Xaero's Minimap", 'xaero/', 4, true), S('journeymap', 'JourneyMap', 'journeymap/', 0, false, false), S('dh', 'Distant Horizons', 'Distant_Horizons_server_data/', 0, false), S('voxy', 'Voxy', '.voxy/', 0, false), S('litematica', 'Litematica', 'schematics/', 3, true), S('flashback', 'Flashback', 'flashback/', 0, false, false), S('bobby', 'Bobby', '.bobby/', 0, false, false)],
  custom: [S('c0', '', 'config/jei/', 3, true), S('c1', '', 'replay_recordings/', 0, false)],
};
export const keybinds: Keybind[] = [
  { id: 'key.zoomify.zoom', name: 'Zoom (Zoomify)', key: 'C' }, { id: 'key.sprint', name: 'Sprint', key: 'Left Ctrl' }, { id: 'key.sneak', name: 'Sneak', key: 'Left Shift' },
  { id: 'key.inventory', name: 'Inventory', key: 'E' }, { id: 'key.togglePerspective', name: 'Toggle perspective', key: 'F5' },
  { id: 'key.voice_chat', name: 'Push to talk (Voice Chat)', key: 'V' }, { id: 'key.xaero_open_map', name: 'Open map (Xaero)', key: 'M' },
];

const choices: KeyChoice[] = [
  ['key.attack', 'Attack/Destroy', 'Left Button'], ['key.use', 'Use Item/Place Block', 'Right Button'], ['key.forward', 'Walk Forwards', 'W'],
  ['key.jump', 'Jump', 'Space'], ['key.drop', 'Drop Selected Item', 'Q'], ['key.chat', 'Open Chat', 'T'], ['key.playerlist', 'List Players', 'Tab'],
  ['key.screenshot', 'Take Screenshot', 'F2'], ['key.sprint', 'Sprint', 'Left Ctrl'], ['key.sneak', 'Sneak', 'Left Shift'], ['key.inventory', 'Open/Close Inventory', 'E'],
  ['key.togglePerspective', 'Toggle Perspective', 'F5'],
].map(([id, name, key]) => ({ id: id!, name: name!, category: 'Minecraft', key: key!, shared: false }));
choices.push(
  { id: 'key.zoomify.zoom', name: 'Zoom (Zoomify)', category: 'Zoomify', key: 'C', shared: false },
  { id: 'key.voice_chat', name: 'Push to talk (Voice Chat)', category: 'Simple Voice Chat', key: 'V', shared: false },
  { id: 'key.xaero_open_map', name: 'Open map (Xaero)', category: "Xaero's World Map", key: 'M', shared: false },
);

/** The preview's Syncing page: edits change the fixtures, as Go would. */
export const mockSync = {
  groups: (): SyncGroups => sync,
  setEnabled: (id: string, on: boolean) => {
    for (const e of [...sync.files, ...sync.folders, ...sync.mods, ...sync.custom]) if (e.id === id) e.enabled = on;
  },
  addCustom: (path: string, dir: boolean): SyncEntry => {
    const e = S(`c-${sync.custom.length}`, '', path.replace(/\\/g, '/').replace(/\/$/, '') + (dir ? '/' : ''), 0, true);
    sync.custom.push(e);
    return e;
  },
  removeCustom: (id: string) => {
    sync.custom = sync.custom.filter((e) => e.id !== id);
  },
  keybinds: (): Keybind[] => keybinds,
  keyCatalog: (): KeyChoice[] => choices.map((c) => ({ ...c, shared: keybinds.some((k) => k.id === c.id) })),
  addKey: (id: string): Keybind => {
    const c = choices.find((x) => x.id === id)!;
    const k = { id, name: c.name, key: c.key };
    if (!keybinds.some((x) => x.id === id)) keybinds.push(k);
    return k;
  },
  setKey: (id: string, code: string): Keybind => {
    const k = keybinds.find((x) => x.id === id)!;
    k.key = code.replace(/^Key|^Digit/, '').replace(/^Mouse(\d)$/, (_, b: string) => `Mouse ${Number(b) + 1}`);
    return k;
  },
  removeKey: (id: string) => {
    const i = keybinds.findIndex((k) => k.id === id);
    if (i >= 0) keybinds.splice(i, 1);
  },
};

const rt = (id: string, major: number, name: string, source: JavaRuntime['source'], path: string, usedBy: number): JavaRuntime =>
  ({ id, major, name, source, path, usedBy, removable: source === 'Managed', forgettable: source === 'Found', default: false });
export const runtimes: JavaRuntime[] = [
  rt('a1', 21, 'Temurin 21.0.5', 'Managed', 'java/temurin-21.0.5', 9),
  rt('a2', 17, 'Temurin 17.0.13', 'Managed', 'java/temurin-17.0.13', 3),
  rt('a3', 8, 'Temurin 8u432', 'Managed', 'java/temurin-8u432', 1),
  rt('a4', 21, 'OpenJDK 21.0.4', 'System', '/usr/lib/jvm/java-21-openjdk', 0),
  rt('a5', 23, 'GraalVM 23.0.1', 'SDKMAN', '~/.sdkman/candidates/java/23.0.1-graal', 0),
  rt('a6', 17, 'Zulu 17.0.12 (x86)', 'Found', '~/.jdks/zulu-17', 0),
];
/** The preview's runtime actions edit the list above. */
export const mockJava = {
  without: (id: string) => {
    const i = runtimes.findIndex((r) => r.id === id);
    if (i >= 0) runtimes.splice(i, 1);
    return [...runtimes];
  },
  setDefault: (id: string) => {
    for (const r of runtimes) r.default = r.id === id;
    return [...runtimes];
  },
};

const V = (id: string, d: number, items: string[]) => ({ id, date: d, items });
export const modUpdates: ModUpdate[] = [
  { id: 'u1', ref: 'iris-shaders', size: 1_300_000, name: 'Iris Shaders', note: 'Modrinth, release', from: '1.8.1', to: '1.8.3', art: { background: '#b48cf2', initial: 'I' }, versions: [V('1.8.3', date(2025, 2, 28), ['Fixes shadows flickering with Distant Horizons', 'Faster shader pack loading on first launch']), V('1.8.2', date(2025, 1, 30), ['Updated for Sodium 0.6.5', 'Fixes a crash with some AMD drivers'])] },
  { id: 'u2', size: 1_300_000, name: 'Not Enough Animations: Extended Compatibility Edition', note: 'CurseForge, release', from: '1.9.2', to: '1.9.4', art: { background: '#f2a25c', initial: 'N' }, versions: [V('1.9.4', date(2025, 2, 2), ['Fixes crossbow animation in first person']), V('1.9.3', date(2025, 1, 18), ['Adds support for 1.21.4 maps'])] },
  { id: 'u3', size: 1_300_000, name: 'Sodium Extra', note: 'Modrinth, release', from: '0.6.0', to: '0.6.1', art: { background: '#e8e8e8', initial: 'S' }, versions: [V('0.6.1', date(2025, 1, 22), ['Restores the fog toggle', 'Translation updates'])] },
  { id: 'u4', size: 1_300_000, name: 'Zoomify', note: 'CurseForge, release', from: '2.14.1', to: '2.14.2', art: { background: '#f2d45c', initial: 'Z' }, versions: [V('2.14.2', date(2025, 1, 9), ['Smoother scroll zoom on high refresh rate screens'])] },
  { id: 'u5', size: 1_300_000, name: 'YetAnotherConfigLib', note: 'Modrinth, release, needed by Zoomify', from: '3.6.1', to: '3.6.2', art: { background: '#c9b6f2', initial: 'Y' }, versions: [V('3.6.2', date(2025, 1, 5), ['Bug fixes'])] },
];

export const packVersions: PackVersion[] = [
  { id: 'v141', number: '1.4.1', channel: 'release', minecraft: '1.16.5', published: daysAgo(2), current: false, offered: true },
  { id: 'v140', number: '1.4.0', channel: 'release', minecraft: '1.16.5', published: daysAgo(40), current: true, offered: false },
  { id: 'v131', number: '1.3.1', channel: 'release', minecraft: '1.16.5', published: daysAgo(90), current: false, offered: false },
];
export const modpackDiff = {
  from: '1.4.0',
  to: '1.4.1',
  summary: 'Fixes chapter 4 quest rewards, updates Create to 0.3.2g and removes two mods that caused server lag.',
  keptEdits: ['config/jei-client.toml', 'config/create-client.toml', 'options.txt'],
  rows: [
    { name: 'Create Crafts and Additions', version: '1.2.4', kind: 'Added' },
    { name: 'Ponder for KubeJS', version: '1.0.2', kind: 'Added' },
    { name: 'Embeddium', version: '0.3.31', kind: 'Added' },
    { name: 'Create', version: '0.3.2f to 0.3.2g', kind: 'Updated' },
    { name: 'Just Enough Items', version: '7.7.1.153 to 7.8.0.11', kind: 'Updated' },
    { name: 'FTB Quests', version: '1605.3.2 to 1605.3.3', kind: 'Updated' },
    { name: 'KubeJS', version: '1605.3.19 to 1605.3.21', kind: 'Updated' },
    { name: 'Mekanism Tools', version: '10.1.2', kind: 'Removed' },
    { name: 'Clumps', version: '6.0.0.28', kind: 'Removed' },
  ] satisfies ModpackDiffRow[],
};

export const otherLaunchers: OtherLauncher[] = [
  { id: 'prism', name: 'Prism Launcher', art: { background: '#b48cf2', initial: 'P' }, summary: '6 instances', instances: 6 },
  { id: 'modrinth', name: 'Modrinth App', art: { background: '#3ecf8e', initial: 'M' }, summary: '3 instances', instances: 3 },
  { id: 'curseforge', name: 'CurseForge', art: { background: '#f2a25c', initial: 'C' }, summary: '2 instances', instances: 2 },
];

/** RevC: installing Distant Horizons. */
export const installPlan: InstallPlan = {
  planId: 'preview', name: 'Distant Horizons', kind: 'mods', version: '2.3.0-b', size: 4_200_000, mods: 48,
  deps: [
    { id: 'dep-modrinth-fabric-api', name: 'Fabric API', required: true, installed: true, version: '0.116.1', update: '0.110.0', size: 2_300_000, art: { background: '#dbc18e', initial: 'F' } },
    { id: 'dep-modrinth-indium', name: 'Indium', summary: 'improves Sodium compatibility', required: false, installed: false, version: '1.0.36', size: 160_000, art: { background: '#c7a3e8', initial: 'I' } },
  ],
};

/** NewC's Detected card. */
export const importPreview: ImportPreview = {
  token: 'preview', name: 'Cobblemon Official Modpack', version: '1.6.1', minecraft: '1.21.1', loader: 'Fabric', source: 'Modrinth',
  art: { background: '#f2a25c', initial: 'C' },
  stats: { mods: 74, modBytes: 71_000_000, files: 212, folders: ['config'], blocked: 0, foreign: ['mods/cobblemon-extras-hotfix.jar'] },
  warnings: [],
  optional: [],
};

/** DlgBlocked: three files waiting for a manual download. */
export const manualView: ManualView = {
  jobId: 'preview', folder: '~/Downloads', done: false, items: [
    { id: '0', name: 'Waystones', fileName: 'waystones-forge-1.16.5-7.6.4.jar', size: 0, state: 'found', twin: false },
    { id: '1', name: 'Balm', fileName: 'balm-1.16.5-1.6.2.jar', size: 312_000, state: 'waiting', twin: true, page: 'https://www.curseforge.com/minecraft/mc-mods/balm', art: { background: '#e2a03f', initial: 'B' } },
    { id: '2', name: 'Controlling', fileName: 'Controlling-forge-1.16.5-7.0.0.jar', size: 88_000, state: 'waiting', twin: false, page: 'https://www.curseforge.com/minecraft/mc-mods/controlling', art: { background: '#9aa3ad', initial: 'C' } },
  ],
};

/** The preview's export contents: the design board's sample instance. */
const MODS: ExportEntry[] = [
  { name: 'Sodium', detail: '0.6.5', bytes: 1_100_000, source: 'modrinth' },
  { name: 'Lithium', detail: '0.14.3', bytes: 712_000, source: 'modrinth' },
  { name: 'Iris Shaders', detail: '1.8.1', bytes: 2_400_000, source: 'modrinth' },
  { name: 'Mod Menu', detail: '13.0.0', bytes: 540_000, source: 'modrinth' },
  { name: "Xaero's Minimap", detail: '25.0.2', bytes: 1_900_000, source: 'curseforge' },
  { name: 'Just Enough Items', detail: '19.21.0', bytes: 1_300_000, source: 'curseforge' },
];
const OWN: ExportEntry[] = [{ name: 'my-tweaks.jar', bytes: 48_000 }, { name: 'OptiLeaves-local.jar', bytes: 96_000 }];
const OFF: ExportEntry[] = [{ name: 'Zoomify', detail: '2.14.2', bytes: 204_000 }];
const EXTRA: ExportEntry[] = [{ name: 'config/', detail: '48 files', bytes: 312_000 }, { name: 'options.txt', bytes: 6_000 }, { name: 'servers.dat', bytes: 1_000 }];
export function exportPreview(plan: ExportPlan): ExportPreview {
  const sum = (l: ExportEntry[]) => l.reduce((n, e) => n + e.bytes, 0);
  const none = { linked: [], bundled: [], listed: [], unlisted: [], off: OFF };
  if (plan.format === 'list') return { ...none, listed: MODS, unlisted: OWN, bytes: plan.list === 'json' ? 2_900 : 1_100 };
  const extra = [...(plan.include.config ? [EXTRA[0]!] : []), ...(plan.include.options ? [EXTRA[1]!] : []), ...(plan.include.servers ? [EXTRA[2]!] : [])];
  const own = plan.format === 'mrpack' ? 'modrinth' : 'curseforge';
  const linked = plan.format === 'zip' ? [] : MODS.filter((m) => m.source === own);
  const bundled = [...MODS.filter((m) => !linked.includes(m)).map(({ source: _, ...m }) => m), ...OWN, ...extra];
  return { ...none, linked, bundled, bytes: sum(bundled) };
}

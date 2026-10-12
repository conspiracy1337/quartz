import type { JavaRuntime, Keybind, ModpackDiffRow, ModUpdate, OtherLauncher, SyncEntry, SyncGroups } from '../types';
import { date } from './time';

const S = (id: string, name: string, path: string, users: number, enabled: boolean, available = true): SyncEntry => ({ id, name, path, users, enabled, available });

export const sync: SyncGroups = {
  files: [S('options', 'Game options', 'options.txt', 8, true), S('servers', 'Server list', 'servers.dat', 12, true), S('history', 'Command history', 'command_history.txt', 5, true), S('hotbar', 'Hotbars', 'hotbar.nbt', 12, true)],
  folders: [S('saves', 'Worlds', 'saves/', 2, false), S('config', 'Configs', 'config/', 0, false), S('screenshots', 'Screenshots', 'screenshots/', 6, true), S('resourcepacks', 'Resource packs', 'resourcepacks/', 9, true), S('shaderpacks', 'Shader packs', 'shaderpacks/', 4, true), S('datapacks', 'Data packs', 'datapacks/', 0, false)],
  mods: [S('xaero', "Xaero's Minimap", 'xaero/', 4, true), S('dh', 'Distant Horizons', 'Distant_Horizons_server_data/', 0, false), S('voxy', 'Voxy', '.voxy/', 0, false), S('litematica', 'Litematica', 'schematics/', 3, true), S('flashback', 'Flashback', 'flashback/', 0, false, false), S('bobby', 'Bobby', '.bobby/', 0, false, false)],
  custom: [S('c0', '', 'config/jei/', 3, true), S('c1', '', 'journeymap/data/', 0, false)],
};
export const totalInstances = 12;

export const keybinds: Keybind[] = [
  { name: 'Zoom (Zoomify)', key: 'C' }, { name: 'Sprint', key: 'Left Ctrl' }, { name: 'Sneak', key: 'Left Shift' }, { name: 'Inventory', key: 'E' },
  { name: 'Toggle perspective', key: 'F5' }, { name: 'Push to talk (Voice Chat)', key: 'V' }, { name: 'Open map (Xaero)', key: 'M' },
];

export const runtimes: JavaRuntime[] = [
  { major: 21, name: 'Temurin 21.0.5', source: 'Managed', path: 'java/temurin-21.0.5', usedBy: 9 },
  { major: 17, name: 'Temurin 17.0.13', source: 'Managed', path: 'java/temurin-17.0.13', usedBy: 3 },
  { major: 8, name: 'Temurin 8u432', source: 'Managed', path: 'java/temurin-8u432', usedBy: 1 },
  { major: 21, name: 'OpenJDK 21.0.4', source: 'System', path: '/usr/lib/jvm/java-21-openjdk', usedBy: 0 },
  { major: 23, name: 'GraalVM 23.0.1', source: 'SDKMAN', path: '~/.sdkman/candidates/java/23.0.1-graal', usedBy: 0 },
  { major: 17, name: 'Zulu 17.0.12 (x86)', source: 'Found', path: '~/.jdks/zulu-17', usedBy: 0 },
];

const V = (id: string, d: number, items: string[]) => ({ id, date: d, items });
export const modUpdates: ModUpdate[] = [
  { name: 'Iris Shaders', note: 'Modrinth, release', from: '1.8.1', to: '1.8.3', art: { background: '#b48cf2', initial: 'I' }, versions: [V('1.8.3', date(2025, 2, 28), ['Fixes shadows flickering with Distant Horizons', 'Faster shader pack loading on first launch']), V('1.8.2', date(2025, 1, 30), ['Updated for Sodium 0.6.5', 'Fixes a crash with some AMD drivers'])] },
  { name: 'Not Enough Animations: Extended Compatibility Edition', note: 'CurseForge, release', from: '1.9.2', to: '1.9.4', art: { background: '#f2a25c', initial: 'N' }, versions: [V('1.9.4', date(2025, 2, 2), ['Fixes crossbow animation in first person']), V('1.9.3', date(2025, 1, 18), ['Adds support for 1.21.4 maps'])] },
  { name: 'Sodium Extra', note: 'Modrinth, release', from: '0.6.0', to: '0.6.1', art: { background: '#e8e8e8', initial: 'S' }, versions: [V('0.6.1', date(2025, 1, 22), ['Restores the fog toggle', 'Translation updates'])] },
  { name: 'Zoomify', note: 'CurseForge, release', from: '2.14.1', to: '2.14.2', art: { background: '#f2d45c', initial: 'Z' }, versions: [V('2.14.2', date(2025, 1, 9), ['Smoother scroll zoom on high refresh rate screens'])] },
  { name: 'YetAnotherConfigLib', note: 'Modrinth, release, needed by Zoomify', from: '3.6.1', to: '3.6.2', art: { background: '#c9b6f2', initial: 'Y' }, versions: [V('3.6.2', date(2025, 1, 5), ['Bug fixes'])] },
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
  { name: 'Prism Launcher', art: { background: '#b48cf2', initial: 'P' }, summary: '6 instances, 2 accounts', instances: 6 },
  { name: 'Modrinth App', art: { background: '#3ecf8e', initial: 'M' }, summary: '3 instances', instances: 3 },
  { name: 'CurseForge', art: { background: '#f2a25c', initial: 'C' }, summary: '2 modpacks', instances: 2 },
];

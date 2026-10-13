import type { BrowseQuery, BrowseResults, DocBlock, ProjectHit, ProjectKind, ProjectPage, ProjectVersion, ProjectType, Span, VersionNotes } from '../types';
import { daysAgo, date } from './time';

const P = (name: string, author: string, summary: string, tint: string, mr: [number, number] | null, cf: [number, number] | null): ProjectHit => {
  const id = name.toLowerCase().replace(/[^a-z0-9]+/g, '-');
  return {
    id, ref: id, name, author, summary, art: { background: tint, initial: name[0]! },
    ...(mr ? { modrinth: { downloads: mr[0] * 1e6, updated: daysAgo(mr[1]) } } : {}),
    ...(cf ? { curseforge: { downloads: cf[0] * 1e6, updated: daysAgo(cf[1]) } } : {}),
  };
};

export const catalog: Record<ProjectType, { categories: string[]; hits: ProjectHit[] }> = {
  Mods: { categories: ['Optimization', 'Utility', 'Decoration', 'Adventure', 'Technology', 'Library'], hits: [
    P('Sodium', 'CaffeineMC', 'A modern rendering engine that greatly improves frame rates.', '#e8e8e8', [58.7, 5], [41.2, 12]),
    P('Jade', 'Snownee', 'Shows what you are looking at: block, entity, health and more.', '#f2d45c', [22.4, 9], [96.3, 3]),
    P('ImmediatelyFast', 'RaphiMC', 'Speeds up entity, text, HUD and map rendering.', '#7cc7e8', [18.2, 3], null),
    P('FerriteCore', 'malte0811', 'Cuts memory use by sharing identical block state data.', '#c9a46b', [41.0, 14], [38.6, 14]),
    P('Xaero’s Minimap', 'xaero96', 'A minimap with waypoints that works on vanilla servers.', '#9be37a', null, [88.1, 6]),
    P('ModernFix', 'embeddedt', 'Faster loading, lower memory use and many bug fixes.', '#e86f6f', [12.4, 7], [9.8, 21]),
  ] },
  Modpacks: { categories: ['Adventure', 'Technology', 'Magic', 'Quests', 'Lightweight', 'Multiplayer'], hits: [
    P('Fabulously Optimized', 'robotkoer', 'Better performance and graphics with a vanilla feel.', '#3ecf8e', [14.0, 4], [3.1, 9]),
    P('Cobblemon Official Modpack', 'Cobblemon', 'Catch, battle and trade creatures in survival.', '#f2a25c', [2.6, 11], null),
    P('All the Mods 10', 'ATMTeam', 'A kitchen-sink pack with tech, magic and exploration.', '#7cc7e8', null, [3.4, 2]),
    P('Prominence II: Hasturian Era', 'Elitemastereric', 'An RPG pack with dungeons, bosses and quests.', '#b48cf2', [1.1, 30], [5.2, 6]),
  ] },
  'Resource packs': { categories: ['Vanilla-like', 'Realistic', 'Simplistic', 'Themed', 'GUI'], hits: [
    P('Fresh Animations', 'FreshLX', 'Animated mob models with new idle and walk motion.', '#7fb069', [9.3, 20], [4.1, 20]),
    P('Stay True', 'haubna', 'Vanilla textures with more variety and depth.', '#9ec9f0', [3.8, 40], [2.2, 40]),
    P('Better Leaves', 'Motschen', 'Bushy leaves for every tree type.', '#4f8f3a', [6.4, 8], null),
  ] },
  Shaders: { categories: ['Realistic', 'Fantasy', 'Lightweight', 'Cartoon'], hits: [
    P('Complementary Reimagined', 'EminGT', 'Balanced look and speed, with many options.', '#f2b36b', [15.2, 18], [8.9, 18]),
    P('BSL Shaders', 'capttatsu', 'Bright, clean visuals with soft shadows.', '#9ec9f0', null, [22.0, 90]),
    P('MakeUp Ultra Fast', 'XavierFST', 'Shaders that run well on low-end GPUs.', '#6bb7d9', [4.4, 12], null),
  ] },
  'Data packs': { categories: ['Worldgen', 'Mobs', 'Utility', 'Game mechanics'], hits: [
    P('Terralith', 'Stardust Labs', 'Over 95 new biomes using only vanilla blocks.', '#7fb069', [7.7, 25], [6.1, 25]),
    P('Incendium', 'Stardust Labs', 'A Nether overhaul with new structures and bosses.', '#b8432f', [2.9, 33], [2.4, 33]),
  ] },
};

const LABELS: Record<ProjectKind, ProjectType> = { mod: 'Mods', modpack: 'Modpacks', resourcepack: 'Resource packs', shader: 'Shaders', datapack: 'Data packs' };

const total = (p: ProjectHit) => (p.modrinth?.downloads ?? 0) + (p.curseforge?.downloads ?? 0);
const newest = (p: ProjectHit) => Math.max(p.modrinth?.updated ?? 0, p.curseforge?.updated ?? 0);

/** The preview's search: the design's rows, filtered by name and sorted. */
export function search(q: BrowseQuery): BrowseResults {
  const text = q.text.trim().toLowerCase();
  const rows = catalog[LABELS[q.type]].hits
    .filter((p) => p.name.toLowerCase().includes(text))
    .sort((a, b) => (q.sort === 'downloads' ? total(b) - total(a) : q.sort === 'updated' ? newest(b) - newest(a) : 0));
  return { searchId: 'preview', rows, changed: [], more: false };
}

export const categories = (kind: ProjectKind) => catalog[LABELS[kind]].categories;

const t = (text: string): Span[] => text.split('`').map((part, i) => (i % 2 ? { text: part, code: true } : { text: part })).filter((s) => s.text);

const notes = [
  ['Fixes shadows flickering with Distant Horizons', 'Faster shader pack loading on first launch', 'Updated for Sodium 0.6.5'],
  ['Fixes a crash when resizing the window with a shader pack on', 'Hand lighting matches OptiFine packs again'],
  ['Supports Minecraft 1.21.4', 'Shader pack settings keep their order'],
];
const INSTALLED = '1.8.1+mc1.21.4';
const PV = (number: string, channel: ProjectVersion['channel'], minecraft: string, published: number, mb: number, i: number): ProjectVersion =>
  ({ id: number, number, channel, minecraft, published, bytes: mb * 1e6, action: number === INSTALLED ? '' : i < 2 ? 'Update' : 'Switch' });
const VERSIONS: ProjectVersion[] = [
  PV('1.8.3+mc1.21.4', 'Release', '1.21.4', date(2025, 2, 28), 2.4, 0),
  PV('1.8.2+mc1.21.4', 'Release', '1.21.4', date(2025, 1, 30), 2.4, 1),
  PV('1.8.1+mc1.21.4', 'Release', '1.21.4', date(2024, 12, 12), 2.3, 2),
  PV('1.8.0+mc1.21.4', 'Release', '1.21.4', date(2024, 12, 4), 2.3, 3),
  PV('1.8.0-beta.8+mc1.21.4', 'Beta', '1.21.4', date(2024, 11, 30), 2.3, 4),
  PV('1.8.0-beta.7+mc1.21.3', 'Beta', '1.21.3', date(2024, 11, 2), 2.2, 5),
  PV('1.7.6+mc1.21.1', 'Release', '1.21, 1.21.1', date(2024, 9, 18), 2.1, 6),
];

/** The preview's Versions tab (the design's Iris list). */
export const versions = (compatible: boolean) => (compatible ? VERSIONS.filter((v) => v.minecraft.includes('1.21.4')) : VERSIONS);

/** The preview's changelogs. */
export const changelog = (ids: string[]): VersionNotes[] =>
  VERSIONS.map((v, i) => ({ id: v.id, number: v.number, published: v.published, items: notes[i % notes.length]! }))
    .filter((n) => (ids.length ? ids.includes(n.id) : true)).slice(0, ids.length ? 5 : 4);

const body: DocBlock[] = [
  { type: 'p', spans: t('Iris is a modern shaders mod for Minecraft, designed to be compatible with existing OptiFine shader packs while performing well with Sodium.') },
  { type: 'h2', spans: t('Features') },
  { type: 'ul', items: ['Loads most OptiFine shader packs without changes', 'Works with Sodium for high frame rates', 'Change shader settings in game, with live preview', 'Shadow rendering that respects Sodium culling'].map(t) },
  { type: 'h2', spans: t('Installing') },
  { type: 'p', spans: t('Iris needs `Sodium` 0.6 or newer. The launcher adds it for you when you install Iris.') },
];
const gallery = [
  ['#f2b36b 0 38%,#b85d4a 38% 48%,#2f4a3a 48% 78%,#1b2622 78%', 'Sunset with soft shadows'],
  ['#7fb7d9 0 45%,#3f7d4f 45% 70%,#2c5236 70%', 'Water reflections at noon'],
  ['#3a2f5c 0 40%,#d97a8a 40% 52%,#1f3a3a 52% 80%,#0f1d1d 80%', 'Night sky with volumetric fog'],
  ['#9ec9f0 0 48%,#7d9b4f 48% 74%,#3f5a3f 74%', 'Shader settings menu'],
  ['#c9d6e3 0 42%,#eef3f7 42% 72%,#8fa1b3 72%', 'Snow biome with shadows on'],
].map(([bg, title]) => ({ art: `linear-gradient(175deg,${bg})`, title: title! }));
const links = [
  { icon: 'github', label: 'Source code', url: 'https://github.com/IrisShaders/Iris' },
  { icon: 'issue', label: 'Issues', url: 'https://github.com/IrisShaders/Iris/issues' },
  { icon: 'globe', label: 'Website', url: 'https://irisshaders.dev' },
];
const side = (downloads: number, days: number, url: string) => ({
  name: 'Iris Shaders', author: 'IrisShaders', summary: 'A shaders mod for Fabric, compatible with OptiFine shader packs.',
  art: { background: '#b48cf2', initial: 'I' }, downloads, updated: daysAgo(days), url, versions: 142,
  badges: ['Client', 'Fabric', 'Quilt', 'NeoForge', 'LGPL-3.0'], body, gallery, links,
});

/** The Iris project page from the design. */
export const iris: ProjectPage = {
  ref: 'iris-shaders', type: 'mod', name: 'Iris Shaders', fresher: 'modrinth',
  modrinth: side(31.6e6, 5, 'https://modrinth.com/mod/iris'),
  curseforge: side(12.9e6, 12, 'https://www.curseforge.com/minecraft/mc-mods/irisshaders'),
  installed: { item: 'iris', source: 'modrinth', versionId: INSTALLED, version: INSTALLED, published: date(2024, 12, 12) },
};

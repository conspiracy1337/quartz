import type { Art, ContentItem, ContentKind, Source } from '../types';

const g = (a: string, b: string): Art => ({ background: `linear-gradient(160deg,${a},${b})` });
const sky = (a: string, b: string, c: string): Art => ({ background: `linear-gradient(175deg,${a} 0 45%,${b} 45% 72%,${c} 72%)` });
const tile = (tint: string, initial: string): Art => ({ background: tint, initial });
const src = (s: string): Source | undefined => ({ M: 'modrinth', C: 'curseforge', F: 'file' } as const)[s as 'M'];

let n = 0;
const R = (name: string, sub: string, meta: string, art: Art, s: string, extra: Partial<ContentItem> = {}): ContentItem => {
  const source = src(s);
  return { id: String(++n), name, sub, meta, art, enabled: true, ...(source ? { source } : {}), ...extra };
};

export const content: Record<ContentKind, ContentItem[]> = {
  mods: [
    R('Sodium', 'CaffeineMC', '0.6.5', tile('#e8e8e8', 'S'), 'M', { clientOnly: true }),
    R('Lithium', 'CaffeineMC', '0.14.3', tile('#9be37a', 'L'), 'M'),
    R('Iris Shaders', 'IrisShaders', '1.8.1', tile('#b48cf2', 'I'), 'M', { status: { kind: 'update', version: '1.8.3' }, clientOnly: true }),
    R('Fabric API', 'FabricMC', '0.114.0', tile('#dbc18e', 'F'), 'M'),
    R('Not Enough Animations: Extended Compatibility Edition', 'tr7zw and contributors', '1.9.2', tile('#f2a25c', 'N'), 'C', { status: { kind: 'update', version: '1.9.4' }, clientOnly: true }),
    R('Entity Culling', 'tr7zw', '1.7.2', tile('#7cc7e8', 'E'), 'M', { enabled: false, clientOnly: true }),
    R('Distant Horizons', 'James Seibel', '2.3.0-b', tile('#5bc0be', 'D'), 'M', { status: { kind: 'locked' } }),
    R('my-local-tweaks', 'Not identified', '1.0', tile('#5a5a63', '?'), 'F', { status: { kind: 'unknown' } }),
  ],
  resourcepacks: [
    R('Fresh Animations', 'Animated mob models', '1.9.3', g('#7fb069', '#3d5a3a'), 'M'),
    R('Better Leaves', 'Bushy leaves for every tree', '9.4', g('#4f8f3a', '#2b4a22'), 'M'),
    R('Faithful 32x', 'The classic look at double resolution', '1.20.1', g('#c9a46b', '#6b5232'), 'C', { status: { kind: 'outdated', label: 'Made for 1.20' } }),
    R('Stay True', 'Vanilla textures, more variety', '1.21.4', g('#9ec9f0', '#3f7d4f'), 'M'),
    R('Low On Fire', 'Lowers the fire overlay', '1.0', g('#e8823a', '#7a2e12'), 'M', { enabled: false }),
    R('Default Dark Mode', 'Dark menus and UI', '1.21.4', g('#3a3a40', '#16161a'), 'M', { enabled: false }),
  ],
  shaders: [
    R('Complementary Reimagined', 'EminGT', 'r5.4', sky('#f2b36b', '#2f4a3a', '#1b2622'), 'M'),
    R('BSL Shaders', 'capttatsu', 'v10.0', sky('#9ec9f0', '#3f7d4f', '#24402b'), 'C', { enabled: false }),
    R('Bliss', 'X0nk', 'v2.1', sky('#3a2f5c', '#d97a8a', '#1f3a3a'), 'M', { enabled: false }),
    R('MakeUp Ultra Fast', 'XavierFST', '9.1c', sky('#6bb7d9', '#2f7fb0', '#e6d39a'), 'M', { enabled: false }),
  ],
  datapacks: [
    R('Terralith', 'Stardust Labs', '2.5.8', g('#7fb069', '#5a4426'), 'M', { status: { kind: 'world', world: 'Northwatch' }, group: 'Northwatch' }),
    R('Incendium', 'Stardust Labs', '5.4.4', g('#b8432f', '#3a1414'), 'M', { status: { kind: 'world', world: 'Northwatch' }, group: 'Northwatch' }),
    R('Armored Elytra', 'VanillaTweaks', '1.21', g('#9aa3ad', '#3a3a40'), 'F', { status: { kind: 'world', world: 'Northwatch' }, group: 'Northwatch' }),
    R('Graves', 'VanillaTweaks', '1.21', g('#6d7f9e', '#2a2a40'), 'F', { status: { kind: 'world', world: 'Creative Plots' }, group: 'Creative Plots', enabled: false }),
  ],
  worlds: [
    R('Northwatch', 'Yesterday, 1.8 GB', 'Survival', sky('#7fb7d9', '#3f7d4f', '#2c5236'), '', { action: 'Play', group: 'Survival' }),
    R('Creative Plots', '2 weeks ago, 240 MB', 'Creative', sky('#9ab3f0', '#6d7f9e', '#3e4a5e'), '', { action: 'Play', group: 'Creative' }),
    R('Shader Test Flatland', '1 month ago, 12 MB', 'Creative', sky('#f0c98a', '#7d9b4f', '#5a4426'), '', { action: 'Play', group: 'Creative' }),
    R('The Long Winter', '2 months ago, 920 MB', 'Hardcore', sky('#c9d6e3', '#eef3f7', '#8fa1b3'), '', { action: 'Play', group: 'Hardcore' }),
    R('Island Speedrun Practice with a Very Long Name', '3 months ago, 88 MB', 'Survival', sky('#6bb7d9', '#2f7fb0', '#e6d39a'), '', { action: 'Play', group: 'Survival' }),
    R('Nether Hub', '5 months ago, 410 MB', 'Survival', sky('#5a1f1f', '#b8432f', '#3a1414'), '', { action: 'Play', group: 'Survival' }),
  ],
  servers: [
    R('Friends SMP', 'smp.kestrel.gg', '', g('#3ecf8e', '#1d6a4a'), '', { action: 'Join', online: true, server: { online: true, version: '1.21.4', fit: 'ok', players: { online: 6, max: 20 }, pingMs: 21 } }),
    R('Hypixel', 'mc.hypixel.net', '', g('#f2d45c', '#b8862f'), '', { action: 'Join', online: true, server: { online: true, version: '1.21.4', fit: 'ok', players: { online: 41208 }, pingMs: 38 } }),
    R('Cobblemon Academy', 'play.cobbleacademy.gg', '', g('#f2a25c', '#8f4a1c'), '', { action: 'Join', online: true, server: { online: true, version: '1.21.1', fit: 'minor', players: { online: 312, max: 500 }, pingMs: 64 } }),
    R('Modded Test', 'test.kestrel.gg', '', g('#b48cf2', '#5a3f8f'), '', { action: 'Join', online: true, server: { online: true, version: '1.20.1', fit: 'major', players: { online: 0, max: 10 }, pingMs: 182 } }),
    R('Old Creative Server', 'creative.kestrel.gg', '', g('#3a3a40', '#1a1a1e'), '', { online: false, server: { online: false, version: '', fit: '' } }),
  ],
};

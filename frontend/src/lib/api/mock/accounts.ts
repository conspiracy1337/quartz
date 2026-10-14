import type { Account, Capes, SignInEvent, Skin } from '../types';

// Preview fixtures use Minecraft's default skins as their textures: the dev server serves
// /tex/skin/<key>.png and /tex/default/<key>.png from Go's embedded copies (vite.config.ts).
const minutesAgo = (m: number) => Date.now() - m * 60_000;
const def = (key: string) => ({ key, slim: key.startsWith('slim-') });

export const accounts: Account[] = [
  { id: 'a1', name: 'Kestrel_Vane', kind: 'microsoft', status: 'ok', detail: '', isDefault: true, defaultSkin: def('wide-steve'), skinHash: 'wide-kai', slim: false, refreshedAt: minutesAgo(12) },
  { id: 'a2', name: 'NightOwl_42', kind: 'microsoft', status: 'ok', detail: '', isDefault: false, defaultSkin: def('slim-alex'), skinHash: 'slim-noor', slim: true, refreshedAt: minutesAgo(300) },
  { id: 'a3', name: 'BuilderBea', kind: 'microsoft', status: 'expired', detail: 'Automatic refresh failed: the password was changed', isDefault: false, defaultSkin: def('wide-efe'), skinHash: 'slim-makena', slim: true, refreshedAt: minutesAgo(4000) },
  { id: 'a4', name: 'TestDummy', kind: 'offline', status: 'offline', detail: 'Offline account, for local testing', isDefault: false, defaultSkin: def('slim-zuri'), skinHash: '', slim: false, refreshedAt: 0 },
];

const skin = (hash: string, name: string, days: number): Skin => ({ hash, name, slim: hash.startsWith('slim-'), added: minutesAgo(days * 1440), source: 'file' });

export const skins: Skin[] = [
  skin('wide-kai', 'Kestrel hoodie', 1),
  skin('slim-makena', 'Ember jacket', 2),
  skin('wide-sunny', 'Frost', 3),
  skin('slim-noor', 'Moss ranger', 4),
  skin('wide-ari', 'Night shift', 5),
  skin('slim-efe', 'Desert nomad', 6),
  skin('wide-zuri', 'Lab coat', 7),
  skin('slim-steve', 'Forest guard', 8),
];

export const capes: Capes = {
  worn: 'migrator',
  capes: [
    { id: 'migrator', name: 'Migrator', texture: '', art: 'linear-gradient(180deg,#a6d8ea 0 20%,#7cc0dc 20% 40%,#a6d8ea 40% 60%,#5fa9c9 60%)' },
    { id: 'cherry', name: 'Cherry Blossom', texture: '', art: 'linear-gradient(160deg,#f7c3d4 0 35%,#e991b0 35% 55%,#f7c3d4 55%)' },
    { id: 'vanilla', name: 'Vanilla', texture: '', art: 'linear-gradient(180deg,#ece5d3 0 70%,#3d3a33 70% 78%,#ece5d3 78%)' },
    { id: 'pan', name: 'Pan', texture: '', art: 'linear-gradient(180deg,#b9a7e6 0 50%,#9480d1 50%)' },
  ],
};

/** Preview stand-in for Go's account manager and skin library. */
export const mockAccounts = {
  list: () => accounts,
  addOffline: (name: string): Account => {
    const a: Account = { id: `o${accounts.length + 1}`.padEnd(32, '0'), name, kind: 'offline', status: 'offline', detail: 'Offline account, for local testing', isDefault: !accounts.length, defaultSkin: def('wide-ari'), skinHash: '', slim: false, refreshedAt: 0 };
    accounts.push(a);
    return a;
  },
  setDefault: (id: string) => accounts.forEach((a) => (a.isDefault = a.id === id)),
  remove: (id: string) => {
    const i = accounts.findIndex((a) => a.id === id);
    if (i >= 0) accounts.splice(i, 1);
  },
  microsoftReady: () => true,
  skins: () => skins,
  capes: (_accountId: string) => capes,
  wear: (accountId: string, skinHash: string, capeId: string): Account => {
    const a = accounts.find((x) => x.id === accountId) ?? accounts[0]!;
    a.skinHash = skinHash;
    a.slim = skins.find((x) => x.hash === skinHash)?.slim ?? false;
    capes.worn = capeId;
    return a;
  },
  resetSkin: (accountId: string): Account => {
    const a = accounts.find((x) => x.id === accountId) ?? accounts[0]!;
    a.skinHash = '';
    return a;
  },
  renameSkin: (hash: string, name: string) => {
    const s = skins.find((x) => x.hash === hash);
    if (s) s.name = name;
  },
  setSkinSlim: (hash: string, slim: boolean) => {
    const s = skins.find((x) => x.hash === hash);
    if (s) s.slim = slim;
  },
  removeSkin: (hash: string): Skin => {
    const i = skins.findIndex((x) => x.hash === hash);
    return skins.splice(Math.max(i, 0), 1)[0]!;
  },
  restoreSkin: (s: Skin) => void skins.push(s),
  importSkin: (_from: string): Skin => skins[0]!,
};

/** The preview's stand-in for Go's sign-in events: the design's code and a QR-like pattern. */
export function previewSignIn(mode: 'browser' | 'code' | 'offline'): SignInEvent {
  if (mode !== 'code') return { flow: 1, mode: 'browser', done: false };
  const finder = (x: number, y: number) => {
    for (const [a, b] of [[0, 0], [14, 0], [0, 14]] as const) {
      const dx = x - a, dy = y - b;
      if (dx >= 0 && dy >= 0 && dx <= 6 && dy <= 6) return dx === 0 || dy === 0 || dx === 6 || dy === 6 || (dx >= 2 && dx <= 4 && dy >= 2 && dy <= 4);
    }
    return null;
  };
  let seed = 7;
  const qr = Array.from({ length: 21 }, (_, y) => Array.from({ length: 21 }, (_, x) => {
    seed = (seed * 1103515245 + 12345) % 2147483648;
    return finder(x, y) ?? ((seed >> 16) & 1) === 1;
  }));
  return { flow: 1, mode: 'code', code: 'K7QX2M9P', url: 'https://www.microsoft.com/link', qr, expiresAt: Date.now() + 872_000, done: false };
}

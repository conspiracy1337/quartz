import type { Account, Capes, SignInEvent, Skin, SkinColors } from '../types';

export const palettes: Record<'kestrel' | 'ember' | 'frost' | 'moss', SkinColors> = {
  kestrel: { skin: '#c99a72', hair: '#4a3426', eye: '#2b4f8f', mouth: '#a8775a', top: '#2f6f73', topD: '#25585b', pants: '#2b2f3a', shoes: '#1b1d22' },
  ember: { skin: '#e0b088', hair: '#7a2e1c', eye: '#3a6b35', mouth: '#b9805f', top: '#b8432f', topD: '#8f3324', pants: '#3a3a40', shoes: '#22201e' },
  frost: { skin: '#f0d2b8', hair: '#d9dde4', eye: '#3c79c4', mouth: '#c99c84', top: '#e9eef5', topD: '#c4ccd8', pants: '#3d5a80', shoes: '#28344a' },
  moss: { skin: '#8d6447', hair: '#1c1a17', eye: '#5a3b1e', mouth: '#6d4a33', top: '#5d7a3a', topD: '#4a6230', pants: '#4b3b2a', shoes: '#2a2119' },
};

const withColors = (base: SkinColors, c: Partial<SkinColors>): SkinColors => ({ ...base, ...c });

const minutesAgo = (m: number) => Date.now() - m * 60_000;

export const accounts: Account[] = [
  { id: 'a1', name: 'Kestrel_Vane', kind: 'microsoft', status: 'ok', detail: '', isDefault: true, skin: palettes.kestrel, skinHash: 'kestrel', slim: false, refreshedAt: minutesAgo(12) },
  { id: 'a2', name: 'NightOwl_42', kind: 'microsoft', status: 'ok', detail: '', isDefault: false, skin: palettes.moss, skinHash: 'moss', slim: false, refreshedAt: minutesAgo(300) },
  { id: 'a3', name: 'BuilderBea', kind: 'microsoft', status: 'expired', detail: 'Automatic refresh failed: the password was changed', isDefault: false, skin: palettes.ember, skinHash: 'ember', slim: false, refreshedAt: minutesAgo(4000) },
  { id: 'a4', name: 'TestDummy', kind: 'offline', status: 'offline', detail: 'Offline account, for local testing', isDefault: false, skin: withColors(palettes.frost, { hair: '#3a3a40', skin: '#9aa3ad' }), skinHash: '', slim: false, refreshedAt: 0 },
];

const skin = (hash: string, name: string, colors: SkinColors, days: number): Skin => ({ hash, name, colors, slim: false, added: minutesAgo(days * 1440), source: 'file' });

export const skins: Skin[] = [
  skin('kestrel', 'Kestrel hoodie', palettes.kestrel, 1),
  skin('ember', 'Ember jacket', palettes.ember, 2),
  skin('frost', 'Frost', palettes.frost, 3),
  skin('moss', 'Moss ranger', palettes.moss, 4),
  skin('night', 'Night shift', withColors(palettes.kestrel, { hair: '#111', top: '#23232b', topD: '#1a1a20', pants: '#15151a' }), 5),
  skin('nomad', 'Desert nomad', withColors(palettes.ember, { hair: '#6b4a2a', skin: '#d8a878', top: '#d9b56b', topD: '#b8944f', pants: '#7a5a3a' }), 6),
  skin('lab', 'Lab coat', withColors(palettes.frost, { hair: '#3a2a1a', top: '#f4f4f5', topD: '#d4d4d8', pants: '#2b3a4a' }), 7),
  skin('guard', 'Forest guard', withColors(palettes.moss, { hair: '#3a2a14', skin: '#a87a52', top: '#2f5a2f', topD: '#244824', pants: '#3a2a1a' }), 8),
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
    const a: Account = { id: `o${accounts.length + 1}`.padEnd(32, '0'), name, kind: 'offline', status: 'offline', detail: 'Offline account, for local testing', isDefault: !accounts.length, skin: palettes.frost, skinHash: '', slim: false, refreshedAt: 0 };
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
    const s = skins.find((x) => x.hash === skinHash);
    a.skinHash = skinHash;
    if (s?.colors) a.skin = s.colors;
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

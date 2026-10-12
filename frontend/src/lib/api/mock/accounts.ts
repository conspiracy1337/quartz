import type { Account, Cape, Skin, SkinColors } from '../types';

export const palettes: Record<'kestrel' | 'ember' | 'frost' | 'moss', SkinColors> = {
  kestrel: { skin: '#c99a72', hair: '#4a3426', eye: '#2b4f8f', mouth: '#a8775a', top: '#2f6f73', topD: '#25585b', pants: '#2b2f3a', shoes: '#1b1d22' },
  ember: { skin: '#e0b088', hair: '#7a2e1c', eye: '#3a6b35', mouth: '#b9805f', top: '#b8432f', topD: '#8f3324', pants: '#3a3a40', shoes: '#22201e' },
  frost: { skin: '#f0d2b8', hair: '#d9dde4', eye: '#3c79c4', mouth: '#c99c84', top: '#e9eef5', topD: '#c4ccd8', pants: '#3d5a80', shoes: '#28344a' },
  moss: { skin: '#8d6447', hair: '#1c1a17', eye: '#5a3b1e', mouth: '#6d4a33', top: '#5d7a3a', topD: '#4a6230', pants: '#4b3b2a', shoes: '#2a2119' },
};

const withColors = (base: SkinColors, c: Partial<SkinColors>): SkinColors => ({ ...base, ...c });

export const accounts: Account[] = [
  { id: 'a1', name: 'Kestrel_Vane', kind: 'microsoft', status: 'ok', detail: 'Microsoft account, refreshed automatically 12 minutes ago', isDefault: true, skin: palettes.kestrel },
  { id: 'a2', name: 'NightOwl_42', kind: 'microsoft', status: 'ok', detail: 'Microsoft account, refreshed automatically today', isDefault: false, skin: palettes.moss },
  { id: 'a3', name: 'BuilderBea', kind: 'microsoft', status: 'expired', detail: 'Automatic refresh failed: the password was changed', isDefault: false, skin: palettes.ember },
  { id: 'a4', name: 'TestDummy', kind: 'offline', status: 'offline', detail: 'Offline account, for local testing', isDefault: false, skin: withColors(palettes.frost, { hair: '#3a3a40', skin: '#9aa3ad' }) },
];

export const skins: Skin[] = [
  { id: 'kestrel', name: 'Kestrel hoodie', colors: palettes.kestrel, slim: false, worn: true },
  { id: 'ember', name: 'Ember jacket', colors: palettes.ember, slim: false, worn: false },
  { id: 'frost', name: 'Frost', colors: palettes.frost, slim: false, worn: false },
  { id: 'moss', name: 'Moss ranger', colors: palettes.moss, slim: false, worn: false },
  { id: 'night', name: 'Night shift', colors: withColors(palettes.kestrel, { hair: '#111', top: '#23232b', topD: '#1a1a20', pants: '#15151a' }), slim: false, worn: false },
  { id: 'nomad', name: 'Desert nomad', colors: withColors(palettes.ember, { hair: '#6b4a2a', skin: '#d8a878', top: '#d9b56b', topD: '#b8944f', pants: '#7a5a3a' }), slim: false, worn: false },
  { id: 'lab', name: 'Lab coat', colors: withColors(palettes.frost, { hair: '#3a2a1a', top: '#f4f4f5', topD: '#d4d4d8', pants: '#2b3a4a' }), slim: false, worn: false },
  { id: 'guard', name: 'Forest guard', colors: withColors(palettes.moss, { hair: '#3a2a14', skin: '#a87a52', top: '#2f5a2f', topD: '#244824', pants: '#3a2a1a' }), slim: false, worn: false },
];

export const capes: Cape[] = [
  { id: 'none', name: 'No cape', art: 'repeating-linear-gradient(45deg,var(--line-2) 0 4px,transparent 4px 8px)' },
  { id: 'migrator', name: 'Migrator', art: 'linear-gradient(180deg,#a6d8ea 0 20%,#7cc0dc 20% 40%,#a6d8ea 40% 60%,#5fa9c9 60%)' },
  { id: 'cherry', name: 'Cherry Blossom', art: 'linear-gradient(160deg,#f7c3d4 0 35%,#e991b0 35% 55%,#f7c3d4 55%)' },
  { id: 'vanilla', name: 'Vanilla', art: 'linear-gradient(180deg,#ece5d3 0 70%,#3d3a33 70% 78%,#ece5d3 78%)' },
  { id: 'pan', name: 'Pan', art: 'linear-gradient(180deg,#b9a7e6 0 50%,#9480d1 50%)' },
];

export const wornCape = 'migrator';

/** Preview stand-in for Go's account store. */
export const mockAccounts = {
  list: () => accounts,
  addOffline: (name: string): Account => {
    const a: Account = { id: `o${accounts.length + 1}`.padEnd(32, '0'), name, kind: 'offline', status: 'offline', detail: 'Offline account, for local testing', isDefault: !accounts.length, skin: palettes.frost };
    accounts.push(a);
    return a;
  },
  setDefault: (id: string) => accounts.forEach((a) => (a.isDefault = a.id === id)),
  remove: (id: string) => {
    const i = accounts.findIndex((a) => a.id === id);
    if (i >= 0) accounts.splice(i, 1);
  },
};

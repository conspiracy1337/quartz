import { describe, expect, it } from 'vitest';
import { serverItem, toShot, worldItem } from './game';

const DAY = 86_400_000;

describe('world and server rows', () => {
  it('shows a world as the design does', () => {
    const r = worldItem({ folder: 'New World', name: 'Northwatch', mode: 'Survival', lastPlayed: Date.now() - DAY, bytes: 1_800_000_000, icon: '/tex/world/0123456789abcdef0123456789abcdef.png' });
    expect(r).toMatchObject({ id: 'New World', name: 'Northwatch', sub: 'Yesterday, 1.8 GB', meta: 'Survival', action: 'Play', group: 'Survival' });
    expect(r.art.background).toContain('url(/tex/world/');
    const plain = worldItem({ folder: 'w', name: 'W', mode: 'Creative', lastPlayed: Date.now(), bytes: 12_000_000 });
    expect(plain.art.background).toContain('linear-gradient');
  });

  it('shows servers online, mismatched and offline', () => {
    const base = { index: 0, name: 'Friends SMP', address: 'smp.kestrel.gg', packs: 'prompt', players: 6, max: 20 };
    expect(serverItem({ ...base, online: true, pingMs: 21 })).toMatchObject({ sub: 'smp.kestrel.gg, 6 of 20 online', meta: '21 ms', action: 'Join', online: true });
    expect(serverItem({ ...base, online: true, pingMs: 182, needs: '1.20.1' }).sub).toBe('Needs 1.20.1');
    const off = serverItem({ ...base, online: false, players: 0 });
    expect(off).toMatchObject({ sub: 'Can not connect', meta: 'Offline', status: { kind: 'offline' }, online: false });
    expect(off.action).toBeUndefined();
    expect(serverItem({ ...base, online: true, players: 41208, max: 0, pingMs: 38 }).sub).toBe('smp.kestrel.gg, 41,208 online');
  });

  it('draws screenshots from their thumbnail and keeps the full image', () => {
    const s = toShot({ id: 'a.png', instanceId: 'fo', world: 'Northwatch', title: 'Northwatch', takenAt: 1, file: 'a.png', width: 2560, height: 1440, bytes: 3, image: '/tex/shot/0123456789abcdef0123456789abcdef.png', thumb: '/tex/thumb/0123456789abcdef0123456789abcdef.png' });
    expect(s.art.background).toContain('/tex/thumb/');
    expect(s.image).toContain('/tex/shot/');
  });
});

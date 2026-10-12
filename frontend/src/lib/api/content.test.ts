import { describe, expect, it } from 'vitest';
import { contentApi, contentArt, toItem } from './content';

const row = (extra: Record<string, unknown> = {}) => ({
  id: 'a1b2c3d4e5f60718', kind: 'mods', name: 'Sodium', sub: 'CaffeineMC', version: '0.6.5', source: 'modrinth', enabled: true, locked: false, ...extra,
}) as Parameters<typeof toItem>[0];

describe('content rows', () => {
  it('maps a Go row to the list item the design shows', () => {
    const item = toItem(row({ status: { kind: 'update', version: '0.6.6' }, project: 'https://modrinth.com/project/sodium' }));
    expect(item).toMatchObject({ id: 'a1b2c3d4e5f60718', name: 'Sodium', sub: 'CaffeineMC', meta: '0.6.5', source: 'modrinth', enabled: true,
      status: { kind: 'update', version: '0.6.6' }, project: 'https://modrinth.com/project/sodium' });
    expect(item.locked).toBeUndefined();
  });

  it('maps problems with their tone', () => {
    expect(toItem(row({ status: { kind: 'problem', label: 'Needs Fabric API', tone: 'warn' } })).status).toEqual({ kind: 'problem', label: 'Needs Fabric API', tone: 'warn' });
  });

  it('draws an icon only from a real hash, else a tint and initial', () => {
    const hash = 'ab'.repeat(32);
    expect(contentArt('Sodium', hash).background).toContain(`/tex/content/${hash}.png`);
    expect(contentArt('Sodium', 'x); background: url(evil')).toEqual({ background: expect.stringMatching(/^#/), initial: 'S' });
    expect(contentArt('Sodium').background).toBe(contentArt('Sodium').background);
    expect(contentArt('§§').initial).toBe('?');
  });
});

describe('preview content', () => {
  it('reorders resource packs: the given ones on top, in order, the rest off', async () => {
    const before = await contentApi.list('fo', 'resourcepacks');
    const [a, b] = [before[1]!.id, before[0]!.id];
    await contentApi.setPackOrder('fo', [a, b]);
    const after = await contentApi.list('fo', 'resourcepacks');
    expect(after.slice(0, 2).map((c) => c.id)).toEqual([a, b]);
    expect(after.slice(2).every((c) => !c.enabled)).toBe(true);
  });

  it('removes and undoes', async () => {
    const [first] = await contentApi.list('fo', 'mods');
    const token = await contentApi.remove('fo', 'mods', [first!.id]);
    expect((await contentApi.list('fo', 'mods')).some((c) => c.id === first!.id)).toBe(false);
    await contentApi.undo('fo', token);
    expect((await contentApi.list('fo', 'mods')).some((c) => c.id === first!.id)).toBe(true);
  });
});

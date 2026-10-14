import { describe, expect, it } from 'vitest';
import { instances, mockInstances, trash } from './instances';
import { loaders } from './versions';

describe('preview instance store', () => {
  it('creates modded instances and moves them between versions', () => {
    const made = mockInstances.create({ name: 'Test', art: { background: '#3ecf8e' }, icon: '', minecraft: '1.21.1', loader: 'fabric', loaderVersion: '0.16.10', group: '', sandbox: true, shares: [], clientJar: '' });
    expect(made.kind).toBe('modded');
    expect(made.loader).toEqual({ kind: 'fabric', version: '0.16.10', id: '0.16.10' });
    mockInstances.setVersion(made.id, '1.21.4', '', '');
    expect(made.loader).toBeUndefined();
    expect(made.kind).toBe('vanilla');
  });
  it('trashes and restores', () => {
    const before = instances.length;
    const e = mockInstances.trash('van');
    expect(instances.length).toBe(before - 1);
    expect(trash[0]?.key).toBe(e.key);
    const back = mockInstances.restore(e.key);
    expect(back.id).toBe('van');
    expect(instances.length).toBe(before);
    expect(trash.find((t) => t.key === e.key)).toBeUndefined();
  });
  it('cleans tags', () => {
    expect(mockInstances.setTags('fo', [' pvp ', 'pvp', '', 'builds']).tags).toEqual(['pvp', 'builds']);
  });
});

describe('loader options', () => {
  it('match the shape Go sends: stable builds first, each with a shown name', () => {
    for (const o of loaders) {
      for (const v of [...o.stable, ...o.beta]) expect(v.name).toBeTruthy();
    }
    expect(loaders[0]?.kind).toBe('none');
  });
});

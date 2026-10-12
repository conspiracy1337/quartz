import { describe, expect, it } from 'vitest';
import { groupsFor, registry, search } from './registry';

describe('settings registry', () => {
  it('has unique ids', () => {
    expect(new Set(registry.map((d) => d.id)).size).toBe(registry.length);
  });
  it('groups a tab in order', () => {
    expect(groupsFor('general').map((g) => g.group)).toEqual(['Launcher']);
    expect(groupsFor('downloads').map((g) => g.group)).toEqual(['Downloads', 'Mod updates']);
  });
  it('searches titles', () => {
    expect(search('memory').map((d) => d.id)).toEqual(['memory', 'warnLowMemory', 'suggestMemory']);
    expect(search('   ')).toEqual([]);
  });
});

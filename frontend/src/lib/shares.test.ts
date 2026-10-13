import { describe, expect, it } from 'vitest';
import { sync } from './api/mock/misc';
import { NEW_INSTANCE_SHARES } from './shares';

describe('New instance shares', () => {
  it('name targets the Syncing page lists (or the shared keybinds)', () => {
    const ids = new Set([...sync.files, ...sync.folders].map((e) => e.id));
    for (const s of NEW_INSTANCE_SHARES) expect(s.id === 'keys' || ids.has(s.id), s.label).toBe(true);
  });
});

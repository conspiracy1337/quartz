import { describe, expect, it } from 'vitest';
import type { RestorePoint } from './api/types';
import { changeLabel, pointLeft, pointOf, pointTitle } from './restore';

const now = new Date(2026, 9, 22, 18, 0).getTime();
const point = (at: number, extra: Partial<RestorePoint> = {}): RestorePoint => ({ id: 'p', at, keep: false, manual: false, changes: [], ...extra });

describe('restore points', () => {
  it('are named by the day their changes were made', () => {
    expect(pointTitle(point(now - 3_600_000), now)).toBe("Before today's changes");
    expect(pointTitle(point(now - 86_400_000), now)).toBe("Before yesterday's changes");
    expect(pointTitle(point(new Date(2026, 9, 1).getTime()), now)).toBe('Before changes on October 1');
    expect(pointTitle(point(now, { manual: true }), now)).toBe('Made by you');
  });

  it('list their changes in the past tense', () => {
    expect(changeLabel('updating 12 mods')).toBe('Updated 12 mods');
    expect(changeLabel('moving to Minecraft 1.21.4')).toBe('Moved to Minecraft 1.21.4');
    expect(changeLabel('rolling back updating a')).toBe('Rolled back updating a');
    expect(changeLabel('fixing things')).toBe('Fixing things');
  });

  it('are found by any of their changes', () => {
    const list = [point(now, { id: 'a', changes: [{ id: 'a', at: now, title: '', reason: 'update', undoable: true }, { id: 'b', at: now, title: '', reason: 'update', undoable: true }] })];
    expect(pointOf(list, 'b')?.id).toBe('a');
    expect(pointOf(list, 'z')).toBeUndefined();
  });

  it('count the days they have left', () => {
    expect(pointLeft(point(now, { expires: now + 2.5 * 86_400_000 }), now)).toBe('3 days left');
    expect(pointLeft(point(now), now)).toBe('');
  });
});

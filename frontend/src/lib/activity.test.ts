import { describe, expect, it } from 'vitest';
import { activityFeed } from './activity';
import { hours } from './format';

const used = { java: '21', memory: { min: 1024, max: 4096 }, account: 'Steve', mods: 0 };

describe('activity feed', () => {
  it('merges runs and content changes, newest first', () => {
    const feed = activityFeed(
      [
        { id: 'a', startedAt: 3_000, lengthMin: 128, exitCode: 0, result: 'ok', ...used },
        { id: 'b', startedAt: 1_000, lengthMin: 3, exitCode: 1, result: 'crash', ...used },
      ],
      [{ at: 2, kind: 'updated', title: 'Updated 4 mods', detail: 'Sodium, Lithium' }, { at: 4, kind: 'removed', title: 'Removed Zoomify', detail: '' }],
    );
    expect(feed.map((e) => e.title)).toEqual(['Removed Zoomify', `Played for ${hours(128)}`, 'Updated 4 mods', 'Game crashed after 3 minutes']);
    expect(feed[0]).toMatchObject({ kind: 'change', icon: 'trash', tone: 'muted' });
    expect(feed[2]).toMatchObject({ kind: 'change', icon: 'arrow-up', tone: 'accent' });
    expect(feed[3]).toMatchObject({ kind: 'session', icon: 'warn', tone: 'danger' });
  });
});

describe('session details', () => {
  it('names the world, the crash cause and the screenshots taken', () => {
    const feed = activityFeed(
      [
        { id: 'a', startedAt: 10 * 60_000, lengthMin: 128, exitCode: 0, result: 'ok', world: 'Northwatch', shots: 3, ...used },
        { id: 'b', startedAt: 1_000, lengthMin: 3, exitCode: 1, result: 'crash', crash: { title: 'Entity Culling 1.7.2 failed to load', detail: 'Mixin apply failed in entityculling', report: true }, ...used },
      ],
      [],
    );
    expect(feed.map((e) => [e.title, e.detail])).toEqual([
      ['Took 3 screenshots', 'Northwatch'],
      [`Played for ${hours(128)}`, 'Northwatch, closed normally'],
      ['Game crashed after 3 minutes', 'Mixin apply failed in entityculling'],
    ]);
    expect(feed[0]).toMatchObject({ icon: 'camera', tone: 'muted' });
  });

  it('keeps the snapshot a change can be rolled back with', () => {
    const feed = activityFeed([], [
      { at: 2, kind: 'updated', title: 'Updated 2 mods', detail: '', snapshot: '1800000000000-a1b2' },
      { at: 3, kind: 'rolledback', title: 'Rolled back to before updating 2 mods', detail: 'Sodium', snapshot: '1800000000001-c3d4' },
      { at: 1, kind: 'added', title: 'Added Iris', detail: '' },
    ]);
    expect(feed[0]).toMatchObject({ icon: 'rotate-left', tone: 'muted', snapshot: '1800000000001-c3d4' });
    expect(feed[1]).toMatchObject({ icon: 'arrow-up', snapshot: '1800000000000-a1b2' });
    expect(feed[2]?.snapshot).toBeUndefined();
  });
});

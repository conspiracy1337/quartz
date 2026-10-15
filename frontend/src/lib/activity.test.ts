import { describe, expect, it } from 'vitest';
import { activityFeed } from './activity';
import { hours } from './format';

describe('activity feed', () => {
  it('merges runs and content changes, newest first', () => {
    const feed = activityFeed(
      [
        { id: 'a', startedAt: 3_000, lengthMin: 128, exitCode: 0, result: 'ok' },
        { id: 'b', startedAt: 1_000, lengthMin: 3, exitCode: 1, result: 'crash' },
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
        { id: 'a', startedAt: 10 * 60_000, lengthMin: 128, exitCode: 0, result: 'ok', world: 'Northwatch', shots: 3 },
        { id: 'b', startedAt: 1_000, lengthMin: 3, exitCode: 1, result: 'crash', crash: { title: 'Entity Culling 1.7.2 failed to load', detail: 'Mixin apply failed in entityculling', report: true } },
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
});

import { describe, expect, it } from 'vitest';
import { activityFeed } from './activity';

describe('activity feed', () => {
  it('merges runs and content changes, newest first', () => {
    const feed = activityFeed(
      [
        { id: 'a', startedAt: 3_000, lengthMin: 128, exitCode: 0, result: 'ok' },
        { id: 'b', startedAt: 1_000, lengthMin: 3, exitCode: 1, result: 'crash' },
      ],
      [{ at: 2, kind: 'updated', title: 'Updated 4 mods', detail: 'Sodium, Lithium' }, { at: 4, kind: 'removed', title: 'Removed Zoomify', detail: '' }],
    );
    expect(feed.map((e) => e.title)).toEqual(['Removed Zoomify', 'Played for 2 h 08 m', 'Updated 4 mods', 'Game crashed after 3 minutes']);
    expect(feed[0]).toMatchObject({ kind: 'change', icon: 'trash', tone: 'muted' });
    expect(feed[2]).toMatchObject({ kind: 'change', icon: 'arrow-up', tone: 'accent' });
    expect(feed[3]).toMatchObject({ kind: 'session', icon: 'warn', tone: 'danger' });
  });
});

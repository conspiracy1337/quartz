import { describe, expect, it } from 'vitest';
import type { ProjectHit } from './api/types';
import { mergePage } from './browse';

const hit = (id: string, ref = id): ProjectHit => ({ id, ref, name: id, author: 'a', summary: '' });

describe('mergePage', () => {
  it('updates rows in place and appends new ones', () => {
    const rows = [hit('mr:a'), hit('mr:b')];
    const out = mergePage(rows, { changed: [hit('mr:a', 'mr:a+cf:1')], rows: [hit('cf:2')] });
    expect(out.map((r) => r.id)).toEqual(['mr:a', 'mr:b', 'cf:2']);
    expect(out[0]!.ref).toBe('mr:a+cf:1');
  });
});

import { describe, expect, it } from 'vitest';
import type { ProjectHit } from './api/types';
import { mergePage, resumeBrowse, type BrowseState } from './browse';

const hit = (id: string, ref = id): ProjectHit => ({ id, ref, name: id, author: 'a', summary: '' });

describe('mergePage', () => {
  it('updates rows in place and appends new ones', () => {
    const rows = [hit('mr:a'), hit('mr:b')];
    const out = mergePage(rows, { changed: [hit('mr:a', 'mr:a+cf:1')], rows: [hit('cf:2')] });
    expect(out.map((r) => r.id)).toEqual(['mr:a', 'mr:b', 'cf:2']);
    expect(out[0]!.ref).toBe('mr:a+cf:1');
  });
});

describe('resumeBrowse', () => {
  const last: BrowseState = { type: 'world', search: 'castle', sort: 'downloads', cats: { Adventure: true }, useVersion: false, useLoader: true, side: '', game: '' };
  it('starts fresh when nothing was left', () => {
    expect(resumeBrowse(null)).toBeUndefined();
    expect(resumeBrowse(null, 'mod')).toBeUndefined();
  });
  it('resumes the search last left, on its type or none asked for', () => {
    expect(resumeBrowse(last)).toBe(last);
    expect(resumeBrowse(last, 'world')).toBe(last);
  });
  it('keeps the search but drops the categories on another type', () => {
    expect(resumeBrowse(last, 'shader')).toEqual({ ...last, type: 'shader', cats: {} });
  });
});

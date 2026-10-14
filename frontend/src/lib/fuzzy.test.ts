import { describe, expect, it } from 'vitest';
import { fuzzy, marks, rank } from './fuzzy';

describe('fuzzy', () => {
  it('finds letters in order, not side by side', () => {
    expect(fuzzy('cfu', 'Check for updates')?.at).toEqual([0, 6, 10]);
    expect(fuzzy('xyz', 'Check for updates')).toBeNull();
    expect(fuzzy('dc', 'Create shortcut')).toBeNull();
  });
  it('ranks a substring above scattered letters, and a word start above the middle', () => {
    const sub = fuzzy('update', 'Check for updates')!.score;
    const scattered = fuzzy('cfu', 'Check for updates')!.score;
    expect(sub).toBeGreaterThan(scattered);
    expect(fuzzy('ins', 'Install launcher update')!.score).toBeGreaterThan(fuzzy('ins', 'Show instances')!.score);
  });
});

describe('rank', () => {
  const items = ['Show keyboard shortcuts', 'Create shortcut…', 'Open data folder'];
  it('puts the best label first and keeps order on ties', () => {
    expect(rank(items, 'short', (s) => s).map((r) => r.item)).toEqual(['Create shortcut…', 'Show keyboard shortcuts']);
    expect(rank(items, '', (s) => s).map((r) => r.item)).toEqual(items);
  });
  it('finds items by their extra text, below label matches', () => {
    const out = rank(items, 'files', (s) => s, (s) => (s === 'Open data folder' ? 'Where the files live' : ''));
    expect(out.map((r) => r.item)).toEqual(['Open data folder']);
    expect(out[0]!.at).toEqual([]);
  });
});

describe('marks', () => {
  it('cuts a label into matched and plain runs', () => {
    expect(marks('Check', [0, 1, 4])).toEqual([{ text: 'Ch', hit: true }, { text: 'ec', hit: false }, { text: 'k', hit: true }]);
  });
});

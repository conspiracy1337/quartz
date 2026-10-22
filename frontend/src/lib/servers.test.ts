import { describe, expect, it } from 'vitest';
import { inHomeOrder, type ServerRow } from './servers.svelte';

const row = (id: string): ServerRow => ({ id, name: id, address: id, art: { background: '' }, stats: undefined, instances: [] });
const ids = (rows: ServerRow[]) => rows.map((r) => r.id).join(' ');

describe("Home's server order", () => {
  it('keeps the list as it came without an order', () => {
    expect(ids(inHomeOrder(['a', 'b', 'c'].map(row), undefined))).toBe('a b c');
  });

  it('puts the ordered servers first and the rest after, in the order they came', () => {
    expect(ids(inHomeOrder(['a', 'b', 'c', 'd'].map(row), ['c', 'gone', 'a']))).toBe('c a b d');
  });
});

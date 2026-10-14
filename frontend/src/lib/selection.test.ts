import { describe, expect, it } from 'vitest';
import { pickRows } from './selection';

describe('pickRows', () => {
  const order = ['a', 'b', 'c', 'd', 'e'];
  it('ticks one row, then a range with shift in either direction', () => {
    const picked: Record<string, boolean> = {};
    let anchor = pickRows(picked, order, 'b', true, null, false);
    anchor = pickRows(picked, order, 'd', true, anchor, true);
    expect(Object.keys(picked).filter((k) => picked[k])).toEqual(['b', 'c', 'd']);
    pickRows(picked, order, 'a', false, anchor, true);
    expect(Object.keys(picked).filter((k) => picked[k])).toEqual([]);
  });
  it('treats shift without an anchor as a plain click', () => {
    const picked: Record<string, boolean> = {};
    pickRows(picked, order, 'c', true, null, true);
    expect(picked).toEqual({ c: true });
  });
  it('ignores an anchor no longer shown', () => {
    const picked: Record<string, boolean> = {};
    pickRows(picked, order, 'c', true, 'gone', true);
    expect(picked).toEqual({ c: true });
  });
});

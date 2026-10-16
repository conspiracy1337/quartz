import { describe, expect, it } from 'vitest';
import { browseTarget, lastTouched } from './instances';

describe('lastTouched', () => {
  it('picks the latest of created, opened and played', () => {
    const list = [
      { id: 'played', created: 1, lastPlayed: 50 },
      { id: 'opened', created: 1, lastOpened: 60, lastPlayed: 10 },
      { id: 'created', created: 40 },
    ];
    expect(lastTouched(list)?.id).toBe('opened');
    expect(lastTouched([...list, { id: 'new', created: 70 }])?.id).toBe('new');
  });
  it('is undefined for no instances', () => expect(lastTouched([])).toBeUndefined());
});

describe('browseTarget', () => {
  const list = [
    { id: 'a', created: 10 },
    { id: 'b', created: 20, lastOpened: 30 },
  ];
  it('keeps a hand pick made after the last touch', () => expect(browseTarget(list, { id: 'a', at: 40 })?.id).toBe('a'));
  it('yields to an instance touched after the pick', () => expect(browseTarget(list, { id: 'a', at: 25 })?.id).toBe('b'));
  it('falls back when the pick is gone', () => expect(browseTarget(list, { id: 'x', at: 99 })?.id).toBe('b'));
});

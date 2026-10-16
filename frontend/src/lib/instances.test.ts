import { describe, expect, it } from 'vitest';
import { lastTouched } from './instances';

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

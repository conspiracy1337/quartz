import { describe, expect, it } from 'vitest';
import { follows } from './console';

describe('console', () => {
  it('follows the run it shows, or the first one it hears from', () => {
    expect(follows('', 'a')).toBe(true);
    expect(follows('b', 'b')).toBe(true);
    expect(follows('b', 'a')).toBe(false);
  });
});

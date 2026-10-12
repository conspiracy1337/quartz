import { describe, expect, it } from 'vitest';
import { inkFor } from './prefs.svelte';

describe('inkFor', () => {
  it('picks dark ink on bright accents and light ink on dark ones', () => {
    expect(inkFor('#3ecf8e')).toBe('#04140c');
    expect(inkFor('#e5e7eb')).toBe('#04140c');
    expect(inkFor('#a78bfa')).toBe('#04140c');
    expect(inkFor('#1d4ed8')).toBe('#f5f5f6');
  });
});

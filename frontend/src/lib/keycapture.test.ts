import { describe, expect, it } from 'vitest';
import { keyCode } from './keycapture';

describe('keyCode', () => {
  it('names keys by their position and mouse buttons by number, as Go reads them', () => {
    expect(keyCode({ code: 'ControlLeft', key: 'Control' } as KeyboardEvent)).toBe('ControlLeft');
    expect(keyCode({ code: 'KeyC', key: 'c' } as KeyboardEvent)).toBe('KeyC');
    expect(keyCode({ button: 3 } as MouseEvent)).toBe('Mouse3');
    expect(keyCode({ button: 1 } as MouseEvent)).toBe('Mouse1');
  });
});

import { describe, expect, it } from 'vitest';
import { chordOf, chords, clash, keysOf, sheet } from './shortcuts';

const press = (key: string, o: Partial<KeyboardEvent> = {}) => ({ key, ctrlKey: false, metaKey: false, altKey: false, shiftKey: false, ...o });

describe('shortcuts', () => {
  it('names chords', () => {
    expect(chordOf(press('k', { ctrlKey: true }))).toBe('Ctrl+K');
    expect(chordOf(press('k', { metaKey: true }))).toBe('Ctrl+K');
    expect(chordOf(press('Enter', { ctrlKey: true, shiftKey: true }))).toBe('Ctrl+Shift+Enter');
    expect(chordOf(press('?', { shiftKey: true }))).toBe('?');
    expect(chordOf(press('Control', { ctrlKey: true }))).toBe('');
  });
  it('applies custom chords to editable ones only', () => {
    const c = chords({ palette: 'Ctrl+J', help: 'Ctrl+H' });
    expect(c.palette).toBe('Ctrl+J');
    expect(c.help).toBe('?');
    expect(sheet(c)[0]!.items[0]!.keys).toEqual(['Ctrl', 'J']);
  });
  it('finds clashes and bare keys', () => {
    const c = chords({});
    expect(clash('palette', 'Ctrl+P', c)).toBe('Ctrl+P is already Switch instance');
    expect(clash('palette', 'K', c)).toBe('Shortcuts need Ctrl or Alt');
    expect(clash('palette', 'Ctrl+J', c)).toBe('');
  });
  it('splits key caps', () => {
    expect(keysOf('Ctrl+,')).toEqual(['Ctrl', ',']);
    expect(keysOf('Ctrl++')).toEqual(['Ctrl', '+']);
  });
});

import { describe, expect, it } from 'vitest';
import type { AppInfo } from '../api/system';
import { descText, groupsFor, registry, search } from './registry';

describe('settings registry', () => {
  it('has unique ids', () => {
    expect(new Set(registry.map((d) => d.id)).size).toBe(registry.length);
  });
  it('groups a tab in order', () => {
    expect(groupsFor('general').map((g) => g.group)).toEqual(['Launcher', 'While playing']);
    expect(groupsFor('downloads').map((g) => g.group)).toEqual(['Downloads', 'Mod updates']);
  });
  it('searches titles', () => {
    expect(search('memory').map((d) => d.id)).toEqual(['memory', 'warnLowMemory', 'suggestMemory']);
    expect(search('   ')).toEqual([]);
  });
  it('says what muting needs on each system', () => {
    const mute = registry.find((d) => d.id === 'muteUnfocused')!;
    const on = (os: string, muteUnfocused: boolean) => descText(mute, { os, muteUnfocused } as AppInfo);
    expect(on('linux', false)).toBe('Needs an X or XWayland display and pactl.');
    expect(on('darwin', false)).toBe('Needs macOS 14.2 or later.');
    expect(on('darwin', true)).toMatch(/asks once to allow audio access/);
    expect(on('windows', true)).toBe('Silences a game while you use another window.');
  });
});

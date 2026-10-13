import { describe, expect, it } from 'vitest';
import { defaultSettings } from './api/mock/settings';
import { startRoute } from './boot';

const with_ = (f: (s: ReturnType<typeof defaultSettings>) => void) => {
  const s = defaultSettings();
  f(s);
  return s;
};

describe('start route', () => {
  it('opens setup until it is done', () => {
    const s = with_((s) => (s.general.setupDone = false));
    expect(startRoute(s, { name: 'home' }, true, undefined)).toEqual({ name: 'setup', step: 1 });
    expect(startRoute(s, { name: 'setup', step: 2 }, false, undefined)).toBeNull();
  });
  it('follows Open on startup only for a fresh window', () => {
    const s = with_((s) => (s.general.startPage = 'last-instance'));
    expect(startRoute(s, { name: 'home' }, true, 'abc')).toEqual({ name: 'instance', id: 'abc', tab: 'overview' });
    expect(startRoute(s, { name: 'home' }, true, undefined)).toBeNull();
    expect(startRoute(s, { name: 'browse' }, false, 'abc')).toBeNull();
    expect(startRoute(with_((s) => (s.general.startPage = 'library')), { name: 'home' }, true, undefined)).toEqual({ name: 'library' });
    expect(startRoute(defaultSettings(), { name: 'home' }, true, 'abc')).toBeNull();
  });
});

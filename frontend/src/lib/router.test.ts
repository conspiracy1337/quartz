import { describe, expect, it } from 'vitest';
import { parse, path, type Route } from './router.svelte';

describe('router', () => {
  const routes: Route[] = [
    { name: 'home' },
    { name: 'library' },
    { name: 'new-instance' },
    { name: 'import' },
    { name: 'project', id: 'iris' },
    { name: 'import', open: '0123abcd' },
    { name: 'project', id: 'mr:AANobbMI+cf:394468', tab: 'versions' },
    { name: 'browse' },
    { name: 'browse', type: 'shader' },
    { name: 'browse', type: 'mod', instance: 'fo-1a2b' },
    { name: 'import', ref: 'mr:1KVo5zza', source: 'modrinth' },
    { name: 'settings', tab: 'appearance' },
    { name: 'settings-search', query: 'memory use' },
    { name: 'instance', id: 'fo', tab: 'mods' },
    { name: 'instance', id: 'fo', tab: 'version', sub: 'loader' },
    { name: 'setup', step: 4 },
  ];
  it('round-trips every route', () => {
    for (const r of routes) expect(parse(path(r))).toEqual(r);
  });
  it('falls back safely', () => {
    expect(parse('')).toEqual({ name: 'home' });
    expect(parse('#/nope/x')).toEqual({ name: 'home' });
    expect(parse('#/settings/bogus')).toEqual({ name: 'settings', tab: 'general' });
    expect(parse('#/instance/fo/bogus')).toEqual({ name: 'instance', id: 'fo', tab: 'overview' });
    expect(parse('#/browse/%E0%A4%A')).toEqual({ name: 'project', id: '' });
    expect(parse('#/browse/t/bogus')).toEqual({ name: 'project', id: 't' });
    expect(parse('#/library/import/x/bogus')).toEqual({ name: 'import' });
  });
});

import { describe, expect, it } from 'vitest';
import { sandboxApi } from './sandbox';

const level = (v: Awaited<ReturnType<typeof sandboxApi.view>>, id: string) => v.rows.find((r) => r.id === id)?.level;

describe('the preview sandbox', () => {
  it('follows the presets and turns Custom on a change', async () => {
    let v = await sandboxApi.view('fo');
    expect(v.preset).toBe('standard');
    expect(v.rows.map((r) => r.id)).toEqual(['instance', 'mods', 'libraries', 'java', 'synced', 'internet', 'discord', 'graphics', 'home']);
    v = await sandboxApi.setPreset('fo', 'strict');
    expect([level(v, 'internet'), level(v, 'discord'), level(v, 'synced'), level(v, 'graphics')]).toEqual(['no', 'no', 'ro', 'on']);
    v = await sandboxApi.setLevel('fo', 'internet', 'on');
    expect(v.preset).toBe('custom');
    expect(level(v, 'synced')).toBe('ro');
    v = await sandboxApi.setPreset('fo', 'standard');
    expect(level(v, 'internet')).toBe('on');
  });

  it('adds, changes and removes an allowed folder', async () => {
    let v = await sandboxApi.addFolder('fo');
    const f = v.rows.find((r) => r.folder)!;
    expect(f.level).toBe('ro');
    expect(f.choices).toEqual(['rw', 'ro', 'remove']);
    v = await sandboxApi.setFolder('fo', f.id, 'rw');
    expect(v.rows.find((r) => r.id === f.id)?.level).toBe('rw');
    v = await sandboxApi.setFolder('fo', f.id, 'remove');
    expect(v.rows.some((r) => r.folder)).toBe(false);
  });
});

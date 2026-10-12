import { describe, expect, it, vi } from 'vitest';
import { api } from './api';
import { tasks } from './tasks.svelte';

const settle = () => new Promise((r) => setTimeout(r, 0));

describe('tasks store (preview engine)', () => {
  it('loads active jobs and fetches history once per revision', async () => {
    const history = vi.spyOn(api.downloads, 'history');
    await tasks.load();
    await settle();
    expect(tasks.active.map((j) => j.meta.title)).toEqual(['Cobblemon Official Modpack', 'Fabulously Optimized']);
    expect(tasks.history).toHaveLength(4);
    await tasks.load();
    await settle();
    expect(history).toHaveBeenCalledTimes(1);
  });

  it('runs controls and reflects them', async () => {
    const first = tasks.active[0]!.id;
    await tasks.run('pause', first);
    expect(tasks.active[0]!.state).toBe('paused');
    await tasks.run('resume', first);
    expect(tasks.active[0]!.state).toBe('running');
    await tasks.all('retryAllFailed');
    await settle();
    expect(tasks.history.some((j) => j.state === 'failed')).toBe(false);
    expect(tasks.active.some((j) => j.meta.title === 'Java 21.0.5 runtime')).toBe(true);
    await tasks.all('clearFinished');
    await settle();
    expect(tasks.history).toHaveLength(0);
    await tasks.all('cancelAll');
    expect(tasks.active).toHaveLength(0);
  });
});

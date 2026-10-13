import { describe, expect, it, vi } from 'vitest';
import { api } from './api';
import type { Job } from './api/types';
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

describe('waiting for a job', () => {
  it('resolves with how a finished job ended', async () => {
    const job = (id: string, state: Job['state']) => ({ id, state }) as Job;
    tasks.active = [];
    tasks.history = [job('ok1', 'done'), job('bad1', 'failed')];
    await expect(tasks.until('ok1')).resolves.toBe(true);
    await expect(tasks.until('bad1')).resolves.toBe(false);
    await expect(tasks.until('')).resolves.toBe(true);
  });
});

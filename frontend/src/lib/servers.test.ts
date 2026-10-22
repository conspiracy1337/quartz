import { describe, expect, it, vi } from 'vitest';
import { inHomeOrder, pingClock, type ServerRow, type ServerScope } from './servers.svelte';

const row = (id: string): ServerRow => ({ id, name: id, address: id, art: { background: '' }, stats: undefined, instances: [] });
const ids = (rows: ServerRow[]) => rows.map((r) => r.id).join(' ');

describe("Home's server order", () => {
  it('keeps the list as it came without an order', () => {
    expect(ids(inHomeOrder(['a', 'b', 'c'].map(row), undefined))).toBe('a b c');
  });

  it('puts the ordered servers first and the rest after, in the order they came', () => {
    expect(ids(inHomeOrder(['a', 'b', 'c', 'd'].map(row), ['c', 'gone', 'a']))).toBe('c a b d');
  });
});

describe('the shared ping clock', () => {
  it('pings every table on one beat and keeps the beat across page changes', () => {
    vi.useFakeTimers();
    const scope = () => ({ refresh: vi.fn(() => Promise.resolve()) }) as unknown as ServerScope & { refresh: ReturnType<typeof vi.fn> };
    const home = scope();
    const tab = scope();
    const leaveHome = pingClock.join(home);
    vi.advanceTimersByTime(60_000);
    expect(home.refresh).toHaveBeenCalledTimes(1);
    // Home closes 30 s into the beat and the instance tab opens: it is pinged when the beat comes, 30 s later.
    vi.advanceTimersByTime(30_000);
    leaveHome();
    const leaveTab = pingClock.join(tab);
    vi.advanceTimersByTime(29_000);
    expect(tab.refresh).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1_000);
    expect(tab.refresh).toHaveBeenCalledTimes(1);
    expect(home.refresh).toHaveBeenCalledTimes(1);
    leaveTab();
    vi.useRealTimers();
  });
});

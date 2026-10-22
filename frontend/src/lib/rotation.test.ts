import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Rotation } from './rotation.svelte';

describe('Rotation', () => {
  let n = 3;
  let rot: Rotation;
  let stop: () => void;
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('document', { hidden: false });
    n = 3;
    rot = new Rotation(() => n, 1000);
    stop = rot.start();
  });
  afterEach(() => {
    stop();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('moves on every interval and wraps around', () => {
    vi.advanceTimersByTime(1000);
    expect(rot.at).toBe(1);
    vi.advanceTimersByTime(2000);
    expect(rot.at).toBe(0);
    rot.step(-1);
    expect(rot.at).toBe(2);
  });
  it('holds while paused or hidden', () => {
    rot.paused = true;
    vi.advanceTimersByTime(1000);
    expect(rot.at).toBe(0);
    rot.paused = false;
    vi.stubGlobal('document', { hidden: true });
    vi.advanceTimersByTime(1000);
    expect(rot.at).toBe(0);
  });
  it('stays in range when items go away', () => {
    rot.step(2);
    n = 2;
    expect(rot.at).toBe(0);
    n = 0;
    rot.step(1);
    expect(rot.at).toBe(0);
  });
  it('stops with its component', () => {
    stop();
    vi.advanceTimersByTime(5000);
    expect(rot.at).toBe(0);
  });
});

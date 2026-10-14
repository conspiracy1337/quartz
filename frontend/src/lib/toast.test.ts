import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { session } from './session.svelte';

describe('toasts', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    session.dismissToast();
    while (session.toast) session.dismissToast();
  });
  afterEach(() => vi.useRealTimers());

  it('never replaces an Undo toast; the next one waits its turn', () => {
    session.notify({ tone: 'trash', subject: 'Survival', text: 'to the trash', undo: () => {} });
    session.notify({ subject: 'Everything is up to date', text: '' });
    expect(session.toast?.subject).toBe('Survival');
    vi.advanceTimersByTime(8000);
    expect(session.toast?.subject).toBe('Everything is up to date');
    vi.advanceTimersByTime(8000);
    expect(session.toast).toBeNull();
  });

  it('lets a plain notice give way at once', () => {
    session.notify({ subject: 'Checking for updates', text: 'in every instance…' });
    session.notify({ subject: '3 updates', text: 'in 2 instances' });
    expect(session.toast?.subject).toBe('3 updates');
  });

  it('drops a toast that is already showing or waiting', () => {
    session.notify({ subject: 'Uploaded', text: 'link', action: { label: 'Open', run: () => {} } });
    session.notify({ subject: 'Copied', text: '' });
    session.notify({ subject: 'Copied', text: '' });
    session.dismissToast();
    expect(session.toast?.subject).toBe('Copied');
    session.dismissToast();
    expect(session.toast).toBeNull();
  });

  it('holds while hovered and resumes with the time it had left', () => {
    session.notify({ subject: 'Held', text: '' });
    vi.advanceTimersByTime(5000);
    session.holdToast(true);
    vi.advanceTimersByTime(60_000);
    expect(session.toast?.subject).toBe('Held');
    session.holdToast(false);
    vi.advanceTimersByTime(2999);
    expect(session.toast?.subject).toBe('Held');
    vi.advanceTimersByTime(1);
    expect(session.toast).toBeNull();
  });
});

import { describe, expect, it } from 'vitest';
import { countdown, idle, reduce } from './signin';

describe('sign-in reducer', () => {
  it('follows the newest flow and drops stale events', () => {
    let s = reduce(idle(), { flow: 2, mode: 'code', code: 'QUARTZ42', qr: [[true]], expiresAt: 900_000, done: false });
    expect(s.code).toBe('QUARTZ42');
    s = reduce(s, { flow: 1, mode: 'browser', done: true, error: { reason: 'cancelled', message: 'x', fixUrl: '' } });
    expect(s.error).toBeNull();
    s = reduce(s, { flow: 2, mode: 'code', step: 'Signing in to Xbox Live', done: false });
    expect(s.code).toBe('QUARTZ42');
    expect(s.step).toBe('Signing in to Xbox Live');
    s = reduce(s, { flow: 2, mode: 'code', done: true, accountId: 'abc' });
    expect(s.accountId).toBe('abc');
    s = reduce(s, { flow: 3, mode: 'browser', done: true, error: { reason: 'timeout', message: 'Timed out', fixUrl: '' } });
    expect(s.code).toBe('');
    expect(s.error?.reason).toBe('timeout');
  });

  it('formats the expiry countdown', () => {
    expect(countdown(872_000, 0)).toBe('14:32');
    expect(countdown(0, 5_000)).toBe('0:00');
  });
});

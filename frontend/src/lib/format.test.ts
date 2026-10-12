import { describe, expect, it } from 'vitest';
import { ago, agoShort, bytes, day, hm, hmLoose, hours } from './format';

const now = new Date('2026-10-11T12:00:00').getTime(); // a Sunday
const daysBack = (d: number) => now - d * 86_400_000;

describe('format', () => {
  it('relative days', () => {
    expect(day(now, now)).toBe('today');
    expect(day(daysBack(1), now)).toBe('yesterday');
    expect(day(daysBack(1 + 0), now)).toBe('yesterday');
    expect(day(daysBack(2), now)).toBe('Friday');
    expect(day(daysBack(20), now)).toBe('September 21');
  });
  it('ages', () => {
    expect(agoShort(undefined, now)).toBe('Never');
    expect(agoShort(daysBack(1), now)).toBe('Yesterday');
    expect(agoShort(daysBack(2), now)).toBe('2 d ago');
    expect(agoShort(daysBack(7), now)).toBe('1 w ago');
    expect(agoShort(daysBack(60), now)).toBe('2 mo ago');
    expect(ago(daysBack(14), now)).toBe('2 weeks ago');
    expect(ago(daysBack(30), now)).toBe('1 month ago');
  });
  it('durations and sizes', () => {
    expect(hours(2470)).toBe('41 h');
    expect(hm(128)).toBe('2 h 08 m');
    expect(hm(40)).toBe('40 m');
    expect(hmLoose(700)).toBe('11 h 40 m');
    expect(bytes(2_400_000)).toBe('2.4 MB');
    expect(bytes(240_000_000)).toBe('240 MB');
    expect(bytes(1_800_000_000)).toBe('1.8 GB');
    expect(bytes(412_000)).toBe('412 KB');
  });
});

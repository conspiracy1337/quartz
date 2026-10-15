import { describe, expect, it } from 'vitest';
import { ago, agoShort, bytes, day, errorText, hours, left, rate, sentenceList, since } from './format';

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
    expect(hours(216_006, 'en-US')).toBe('3,600.1 hours');
    expect(hours(216_006, 'de-DE')).toBe('3.600,1 hours');
    expect(hours(2470, 'en-US')).toBe('41.1 hours');
    expect(hours(59, 'en-US')).toBe('0.9 hours');
    expect(hours(0, 'en-US')).toBe('0.0 hours');
    expect(bytes(2_400_000)).toBe('2.4 MB');
    expect(bytes(240_000_000)).toBe('240 MB');
    expect(bytes(1_800_000_000)).toBe('1.8 GB');
    expect(bytes(412_000)).toBe('412 KB');
  });
});

describe('recent times', () => {
  const now = Date.UTC(2026, 9, 12, 12);
  it('since', () => {
    expect(since(now - 20_000, now)).toBe('just now');
    expect(since(now - 5 * 60_000, now)).toBe('5 minutes ago');
    expect(since(now - 60_000, now)).toBe('1 minute ago');
    expect(since(now - 3 * 3_600_000, now)).toBe('3 hours ago');
  });
  it('left and rate', () => {
    expect(left(0)).toBe('');
    expect(left(40)).toBe('40 s left');
    expect(left(150)).toBe('3 min left');
    expect(rate(12_400_000)).toBe('12.4 MB/s');
    expect(rate(240_000_000)).toBe('240 MB/s');
  });
});

describe('errorText', () => {
  it('drops Go package prefixes', () => {
    expect(errorText(new Error('skins: not a Minecraft skin'))).toBe('Not a Minecraft skin');
    expect(errorText('accounts: sign in again: msa: expired')).toBe('Sign in again: msa: expired');
  });
});

describe('sentenceList', () => {
  it('joins names as a sentence', () => {
    expect(sentenceList([])).toBe('');
    expect(sentenceList(['keybinds'])).toBe('Keybinds');
    expect(sentenceList(['keybinds', 'server list', "Xaero's Minimap"])).toBe("Keybinds, server list and Xaero's Minimap");
  });
});

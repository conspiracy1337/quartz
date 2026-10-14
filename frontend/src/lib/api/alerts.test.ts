import { describe, expect, it } from 'vitest';
import { collectAlerts, MAX_ALERTS, rankAlerts } from './alerts';
import type { Alert } from './types';

const missing = (name: string): Alert => ({ tone: 'warn', subject: name, text: 'is missing Sodium', action: 'Fix', target: { kind: 'instance', id: name, tab: 'mods' } });
const disk: Alert = { tone: 'warn', subject: 'Low disk space', text: '3 GB left', action: 'Clean up', target: { kind: 'settings', tab: 'storage' } };
const pack: Alert = { tone: 'info', subject: 'Pack', text: '1.2 is out', action: 'Review', target: { kind: 'modpack-update', id: 'p' } };
const follow: Alert = { tone: 'info', subject: 'Vanilla', text: 'every mod supports 1.21.5', action: 'Switch', target: { kind: 'instance', id: 'v', tab: 'version' } };

describe('rankAlerts', () => {
  it('merges each kind into one alert that acts on the first', () => {
    const [a] = rankAlerts({ missing: [missing('Survival'), missing('Create'), missing('Skyblock')] });
    expect(a).toMatchObject({ subject: 'Survival and 2 more', text: 'are missing mods', target: { id: 'Survival' } });
  });
  it('keeps a single alert as it is', () => {
    expect(rankAlerts({ missing: [missing('Survival')] })).toEqual([missing('Survival')]);
  });
  it('puts the most urgent first and shows at most three', () => {
    const out = rankAlerts({ follow: [follow], pack: [pack], missing: [missing('A')], disk: [disk] });
    expect(out).toHaveLength(MAX_ALERTS);
    expect(out.map((a) => a.subject)).toEqual(['Low disk space', 'A', 'Pack']);
  });
});

describe('collectAlerts', () => {
  it('leaves out a source that fails instead of hiding every alert', async () => {
    const none = () => Promise.resolve([]);
    const out = await collectAlerts({
      'sign-in': () => Promise.reject(new Error('keychain locked')), folder: none, disk: () => Promise.resolve([disk]),
      missing: none, pack: none, follow: none,
    });
    expect(out).toEqual([disk]);
  });
});

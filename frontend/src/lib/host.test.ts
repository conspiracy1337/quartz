import { describe, expect, it } from 'vitest';
import { worldMeta } from './api/host';
import { session } from './session.svelte';

const DAY = 86_400_000;

describe('host', () => {
  it('words world rows as the board does', () => {
    const now = Date.UTC(2026, 9, 13, 12);
    expect(worldMeta('Survival', now - DAY, now)).toBe('Survival, played yesterday');
    expect(worldMeta('Creative', now - 14 * DAY, now)).toBe('Creative, 2 weeks ago');
  });

  it('follows the server from starting to stopped', () => {
    const base = { instanceId: 'fo', runId: 'r1', port: 25565, players: 0, max: 10, address: 'localhost:25565' };
    session.hostChanged({ ...base, state: 'starting', stage: 'Preparing spawn area' });
    expect(session.hosting?.state).toBe('starting');
    session.hostChanged({ ...base, state: 'running', players: 1 });
    expect(session.hosting?.players).toBe(1);
    session.hostChanged({ ...base, state: 'stopped', result: 'failed', message: 'Port in use' });
    expect(session.hosting).toBeNull();
    expect(session.hostShown?.state).toBe('stopped'); // the panel stays, with Start
    session.closeHostPanel();
    expect(session.hostShown).toBeNull();
    expect(session.toast).toMatchObject({ tone: 'error', subject: "Couldn't start the server", text: 'Port in use' });
  });

  it('keeps a newer server when an older one reports stopping', () => {
    const base = { instanceId: 'fo', port: 25565, players: 0, max: 10, address: 'localhost:25565' };
    session.hostChanged({ ...base, runId: 'r2', state: 'running' });
    session.hostChanged({ ...base, runId: 'r1', state: 'stopped', result: 'ok' });
    expect(session.hosting?.runId).toBe('r2');
  });
});

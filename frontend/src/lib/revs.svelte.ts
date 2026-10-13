// Revision counters for data that changes after it was first loaded. A query that
// loads through revs.load(key, …) re-runs whenever that key is bumped: by Go's
// "<key>:changed" event inside the app, or by the screen that made the change.
import { onEvent } from './bridge';

export type RevKey = 'accounts' | 'instances' | 'content' | 'modpacks' | 'manual' | 'worlds' | 'servers' | 'screenshots';
const KEYS: RevKey[] = ['accounts', 'instances', 'content', 'modpacks', 'manual', 'worlds', 'servers', 'screenshots'];

class Revisions {
  accounts = $state(0);
  instances = $state(0);
  content = $state(0);
  modpacks = $state(0);
  manual = $state(0);
  worlds = $state(0);
  servers = $state(0);
  screenshots = $state(0);

  /** Reads the key (so the calling query tracks it), then loads. */
  load<T>(key: RevKey, fn: () => Promise<T>): Promise<T> {
    void this[key];
    return fn();
  }

  bump(key: RevKey) {
    this[key]++;
  }

  /** Subscribes to every "<key>:changed" event; returns the unsubscribe function. */
  listen(): () => void {
    const offs = KEYS.map((k) => onEvent(`${k}:changed`, () => this.bump(k)));
    return () => offs.forEach((off) => off());
  }
}

export const revs = new Revisions();

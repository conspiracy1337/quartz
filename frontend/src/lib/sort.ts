import type { ServerStats } from './api/types';

// Sorting a table by its column titles (SortTitle).

export type Sort<K extends string> = { key: K; dir: 1 | -1 } | null;

/** The sort after a click on key's title: by it, again the other way, a third time back to fallback. */
export function nextSort<K extends string>(cur: Sort<K>, key: K, fallback: Sort<K> = null): Sort<K> {
  return cur?.key !== key ? { key, dir: 1 } : cur.dir === 1 ? { key, dir: -1 } : fallback;
}

export const names = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });
/** Text sorts with empty cells last. */
export const text = (a: string, b: string) => (a && b ? names.compare(a, b) : Number(!a) - Number(!b));

const NONE = 1e9;
type ByStats = (a: ServerStats | undefined, b: ServerStats | undefined) => number;
/** Servers by their ping answer; numbers sort the useful end first (most players, lowest ping). */
export const byStats: Record<'version' | 'players' | 'ping', ByStats> = {
  version: (a, b) => text(a?.online ? a.version : '', b?.online ? b.version : ''),
  players: (a, b) => (b?.players?.online ?? -1) - (a?.players?.online ?? -1),
  ping: (a, b) => (a?.pingMs ?? NONE) - (b?.pingMs ?? NONE),
};

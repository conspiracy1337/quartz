// Small reads over instance lists that several screens share.
import type { InstanceSummary } from './api/types';

type Touched = Pick<InstanceSummary, 'created' | 'lastOpened' | 'lastPlayed'>;

/** When an instance was last created, opened or played, whichever is latest. */
const touched = (i: Touched) => Math.max(i.created, i.lastOpened ?? 0, i.lastPlayed ?? 0);

/** The instance touched last, or undefined for none (Browse's default target). */
export function lastTouched<T extends Touched>(list: readonly T[]): T | undefined {
  let best: T | undefined;
  for (const i of list) if (!best || touched(i) > touched(best)) best = i;
  return best;
}

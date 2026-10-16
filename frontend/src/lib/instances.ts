// Small reads over instance lists that several screens share.
import type { InstanceSummary } from './api/types';

type Touched = Pick<InstanceSummary, 'created' | 'lastOpened' | 'lastPlayed'>;

/** When an instance was last created, opened or played, whichever is latest. */
const touched = (i: Touched) => Math.max(i.created, i.lastOpened ?? 0, i.lastPlayed ?? 0);

/** The instance touched last, or undefined for none. */
export function lastTouched<T extends Touched>(list: readonly T[]): T | undefined {
  let best: T | undefined;
  for (const i of list) if (!best || touched(i) > touched(best)) best = i;
  return best;
}

/** An instance picked by hand to add to, and when (unix ms). */
export type TargetPick = { id: string; at: number };

/** Where Browse and project pages add to: the hand-picked instance, unless one was created,
    opened or played since, which wins as it would without a pick. */
export function browseTarget<T extends Touched & { id: string }>(list: readonly T[], pick: TargetPick | null): T | undefined {
  const last = lastTouched(list);
  const picked = pick ? list.find((i) => i.id === pick.id) : undefined;
  return picked && (!last || pick!.at >= touched(last)) ? picked : last;
}

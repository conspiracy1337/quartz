// Restore points as the rollback dialog, the activity feed and the crash
// dialog name them.
import { day, plural } from './format';
import type { RestorePoint } from './api/types';

/** "Before today's changes", "Before Saturday's changes", "Before changes on March 18"; "Made by you" for one made by hand. */
export function pointTitle(p: RestorePoint, now = Date.now()): string {
  if (p.manual) return 'Made by you';
  const d = day(p.at, now);
  return /^[a-z]+$/i.test(d) ? `Before ${d}'s changes` : `Before changes on ${d}`;
}

// A change is named for "Before …" ("updating 12 mods"); a list of changes reads in the past tense.
const PAST: Record<string, string> = {
  updating: 'Updated', moving: 'Moved', changing: 'Changed', switching: 'Switched', installing: 'Installed',
  removing: 'Removed', adding: 'Added', rolling: 'Rolled', turning: 'Turned', going: 'Went',
};

/** "updating 12 mods" → "Updated 12 mods" (an unknown verb is just capitalised). */
export function changeLabel(title: string): string {
  if (!title) return 'Made by you';
  const [verb = '', ...rest] = title.split(' ');
  const past = PAST[verb.toLowerCase()];
  const head = past ?? verb.charAt(0).toUpperCase() + verb.slice(1);
  return [head, ...rest].join(' ');
}

/** The point a change (by the id its activity entry or toast carries) belongs to. */
export function pointOf(points: RestorePoint[], change: string): RestorePoint | undefined {
  return points.find((p) => p.id === change || p.changes.some((c) => c.id === change));
}

/** "3 days left" for a point that expires; "" for one that doesn't. */
export function pointLeft(p: RestorePoint, now = Date.now()): string {
  if (!p.expires) return '';
  return `${plural(Math.max(1, Math.ceil((p.expires - now) / 86_400_000)), 'day')} left`;
}

// Home's "Needs attention" bar: every source's alerts, most urgent first, with each
// source's alerts merged into one ("Survival and 2 more are missing mods") so the bar
// keeps at most MAX_ALERTS readable columns. A source that fails is left out instead
// of hiding the others.
import type { Alert } from './types';

/** Sources in the order they matter. */
export const ALERT_ORDER = ['sign-in', 'folder', 'disk', 'missing', 'pack', 'follow'] as const;
export type AlertSource = (typeof ALERT_ORDER)[number];

export const MAX_ALERTS = 3;

/** How several alerts of one source read as one; the action opens the first. */
const MERGED: Partial<Record<AlertSource, (first: Alert, more: number) => Pick<Alert, 'subject' | 'text'>>> = {
  'sign-in': (a, n) => ({ subject: `${a.subject} and ${n} more`, text: 'need to sign in again' }),
  missing: (a, n) => ({ subject: `${a.subject} and ${n} more`, text: 'are missing mods' }),
  pack: (a, n) => ({ subject: `${a.subject} and ${n} more`, text: 'have updates' }),
  follow: (a, n) => ({ subject: `${a.subject} and ${n} more`, text: 'can move to the newest release' }),
};

/** One alert per source, most urgent first, at most MAX_ALERTS. */
export function rankAlerts(found: Partial<Record<AlertSource, Alert[]>>): Alert[] {
  const out: Alert[] = [];
  for (const src of ALERT_ORDER) {
    const list = found[src] ?? [];
    const first = list[0];
    if (!first) continue;
    const merge = MERGED[src];
    out.push(list.length > 1 && merge ? { ...first, ...merge(first, list.length - 1) } : first);
  }
  return out.slice(0, MAX_ALERTS);
}

/** Runs every source; one that fails counts as empty (and is logged). */
export async function collectAlerts(sources: Record<AlertSource, () => Promise<Alert[]>>): Promise<Alert[]> {
  const settled = await Promise.allSettled(ALERT_ORDER.map((s) => sources[s]()));
  const found: Partial<Record<AlertSource, Alert[]>> = {};
  settled.forEach((r, i) => {
    const src = ALERT_ORDER[i]!;
    if (r.status === 'fulfilled') found[src] = r.value;
    else console.warn(`Home alerts: ${src} failed`, r.reason);
  });
  return rankAlerts(found);
}

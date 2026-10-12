// The Overview's activity feed: play sessions from the launch history plus the
// content changes Go logs, newest first.
import type { ActivityEvent, ContentChange, Run } from './api/types';
import { hm } from './format';

const MAX = 50;

const minutes = (n: number) => `${n} ${n === 1 ? 'minute' : 'minutes'}`;

/** One launch as a feed entry. */
export function sessionEvent(r: Run): ActivityEvent {
  switch (r.result) {
    case 'crash':
      return { kind: 'session', icon: 'warn', tone: 'danger', title: `Game crashed after ${minutes(r.lengthMin)}`, detail: `Exit code ${r.exitCode}`, at: r.startedAt };
    case 'failed':
      return { kind: 'session', icon: 'warn', tone: 'danger', title: 'The game failed to start', detail: `Exit code ${r.exitCode}`, at: r.startedAt };
    case 'killed':
      return { kind: 'session', icon: 'play', tone: 'muted', title: `Played for ${hm(r.lengthMin, true)}`, detail: 'Stopped from the launcher', at: r.startedAt };
  }
  return { kind: 'session', icon: 'play', tone: 'muted', title: `Played for ${hm(r.lengthMin, true)}`, detail: 'Closed normally', at: r.startedAt };
}

const CHANGE_ICON = { added: 'plus', updated: 'arrow-up', removed: 'trash' } as const;

/** One content change as a feed entry (Go records seconds). */
export function changeEvent(c: ContentChange): ActivityEvent {
  return { kind: 'change', icon: CHANGE_ICON[c.kind], tone: c.kind === 'removed' ? 'muted' : 'accent', title: c.title, detail: c.detail ?? '', at: c.at * 1000 };
}

export function activityFeed(runs: Run[], changes: ContentChange[]): ActivityEvent[] {
  return [...runs.map(sessionEvent), ...changes.map(changeEvent)].sort((a, b) => b.at - a.at).slice(0, MAX);
}

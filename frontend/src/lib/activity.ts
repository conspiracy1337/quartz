// The Overview's activity feed: play sessions from the launch history plus the
// content changes Go logs, newest first.
import type { ActivityEvent, ContentChange, Run } from './api/types';
import { hm } from './format';

const MAX = 50;

const minutes = (n: number) => `${n} ${n === 1 ? 'minute' : 'minutes'}`;

/** "Northwatch, closed normally": the world played (when known) before how it ended. */
const inWorld = (r: Run, how: string) => (r.world ? `${r.world}, ${how.charAt(0).toLowerCase()}${how.slice(1)}` : how);

/** One launch as a feed entry. */
export function sessionEvent(r: Run): ActivityEvent {
  const cause = r.crash?.detail || `Exit code ${r.exitCode}`;
  switch (r.result) {
    case 'crash':
      return { kind: 'session', icon: 'warn', tone: 'danger', title: `Game crashed after ${minutes(r.lengthMin)}`, detail: cause, at: r.startedAt };
    case 'failed':
      return { kind: 'session', icon: 'warn', tone: 'danger', title: 'The game failed to start', detail: cause, at: r.startedAt };
    case 'killed':
      return { kind: 'session', icon: 'play', tone: 'muted', title: `Played for ${hm(r.lengthMin, true)}`, detail: inWorld(r, 'Stopped from the launcher'), at: r.startedAt };
  }
  return { kind: 'session', icon: 'play', tone: 'muted', title: `Played for ${hm(r.lengthMin, true)}`, detail: inWorld(r, 'Closed normally'), at: r.startedAt };
}

/** "Took 3 screenshots" after the session that took them. */
export function shotsEvent(r: Run): ActivityEvent[] {
  if (!r.shots) return [];
  return [{ kind: 'session', icon: 'camera', tone: 'muted', title: `Took ${r.shots} ${r.shots === 1 ? 'screenshot' : 'screenshots'}`, detail: r.world ?? '', at: r.startedAt + r.lengthMin * 60_000 }];
}

const CHANGE_ICON = { added: 'plus', updated: 'arrow-up', removed: 'trash' } as const;

/** One content change as a feed entry (Go records seconds). */
export function changeEvent(c: ContentChange): ActivityEvent {
  return { kind: 'change', icon: CHANGE_ICON[c.kind], tone: c.kind === 'removed' ? 'muted' : 'accent', title: c.title, detail: c.detail ?? '', at: c.at * 1000 };
}

export function activityFeed(runs: Run[], changes: ContentChange[]): ActivityEvent[] {
  return [...runs.map(sessionEvent), ...runs.flatMap(shotsEvent), ...changes.map(changeEvent)].sort((a, b) => b.at - a.at).slice(0, MAX);
}

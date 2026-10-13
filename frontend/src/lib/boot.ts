// Where the launcher opens: first-run setup until it is done, else the "Open on
// startup" page when the window wasn't opened on a page already.
import type { Settings } from './api/system';
import type { Route } from './router.svelte';

/** The route to open with, or null to stay where the window is. */
export function startRoute(s: Settings, current: Route, hashEmpty: boolean, lastInstance: string | undefined): Route | null {
  if (!s.general.setupDone) return current.name === 'setup' ? null : { name: 'setup', step: 1 };
  if (!hashEmpty) return null;
  switch (s.general.startPage) {
    case 'library':
      return { name: 'library' };
    case 'last-instance':
      return lastInstance ? { name: 'instance', id: lastInstance, tab: 'overview' } : null;
  }
  return null;
}

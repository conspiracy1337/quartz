// Snapshots taken before updates and version changes, and rolling back to
// them: Go's snapshot service. The preview keeps its edits in memory.
import { bridged } from '../bridge';
import { revs } from '../revs.svelte';
import * as Go from '../wailsjs/go/api/Snapshots';
import { mockSnapshots as mock } from './mock/snapshots';

/** A change, then everything it may have changed reloads. */
const edit = <A extends unknown[], R>(fn: (...a: A) => Promise<R>) => async (...a: A): Promise<R> => {
  const r = await fn(...a);
  revs.bump('snapshots');
  revs.bump('content');
  revs.bump('instances');
  return r;
};

const list = bridged(Go.List, mock.list);
const diff = bridged(Go.Diff, mock.diff);
const suspect = bridged(Go.Suspect, mock.suspect);

export const snapshotsApi = {
  /** An instance's snapshots that haven't expired, newest first. */
  list: (id: string) => revs.load('snapshots', () => list(id)),
  /** What rolling back to a snapshot would change, row by row. */
  diff: (id: string, snap: string) => revs.load(['snapshots', 'content', 'instances'], () => diff(id, snap)),
  /** The newest change since the game last closed normally (the crash dialog's offer), or null. */
  suspect: (id: string) => revs.load('snapshots', () => suspect(id)),
  create: edit(bridged(Go.Create, mock.create)),
  /** Keeps a snapshot until deleted, or lets it expire 7 days after it was taken. */
  setKeep: edit(bridged(Go.SetKeep, mock.setKeep)),
  remove: edit(bridged(Go.Delete, mock.remove)),
  /** Puts the picked rows (by key) back; the result names the snapshot that undoes it. */
  rollback: edit(bridged(Go.Rollback, mock.rollback)),
  /** Rolls back what the change after a snapshot touched (the Undo toast). */
  undo: edit(bridged(Go.Undo, mock.undo)),
};

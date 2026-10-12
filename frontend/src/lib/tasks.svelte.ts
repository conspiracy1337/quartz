// Live download jobs from Go's job engine. Go pushes a coalesced snapshot on every change
// (tasks:changed); the 24-hour history is refetched only when its revision moves.
import { api } from './api';
import type { Job, TaskSnapshot } from './api/types';
import { onEvent } from './bridge';

type Control = 'pause' | 'resume' | 'cancel' | 'retry';
type BulkControl = 'pauseAll' | 'resumeAll' | 'cancelAll' | 'retryAllFailed' | 'clearFinished';

class Tasks {
  active = $state<Job[]>([]);
  history = $state<Job[]>([]);
  speed = $state(0);
  etaSec = $state(0);
  loaded = $state(false);
  #rev = -1;

  async load() {
    this.apply(await api.downloads.snapshot());
  }

  /** Subscribes to Go's snapshots; returns the unsubscribe function. */
  listen(): () => void {
    return onEvent<TaskSnapshot>('tasks:changed', (s) => this.apply(s));
  }

  apply(s: TaskSnapshot) {
    this.active = s.active ?? [];
    this.speed = s.speed;
    this.etaSec = s.etaSec;
    this.loaded = true;
    if (s.historyRev !== this.#rev) {
      this.#rev = s.historyRev;
      void api.downloads.history().then((h) => (this.history = h ?? []));
    }
  }

  /** Runs a per-job control, then refreshes (the preview has no push events). */
  async run(control: Control, id: string) {
    await api.downloads[control](id);
    await this.load();
  }

  async all(control: BulkControl) {
    await api.downloads[control]();
    await this.load();
  }
}

export const tasks = new Tasks();

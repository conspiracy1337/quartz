// Live download jobs from Go's job engine. Go pushes a coalesced snapshot on every change
// (tasks:changed); the 24-hour history is refetched only when its revision moves.
import { api } from './api';
import type { Job, TaskSnapshot } from './api/types';
import { onEvent } from './bridge';
import { percent, plural, rate } from './format';

type Control = 'pause' | 'resume' | 'cancel' | 'retry';
type BulkControl = 'pauseAll' | 'resumeAll' | 'cancelAll' | 'retryAllFailed' | 'clearFinished';

class Tasks {
  active = $state<Job[]>([]);
  history = $state<Job[]>([]);
  speed = $state(0);
  etaSec = $state(0);
  loaded = $state(false);
  /** The running queue as the status bar and the download panel show it. */
  summary = $derived.by(() => {
    const jobs = this.active;
    const done = jobs.reduce((n, j) => n + j.doneBytes, 0);
    const total = jobs.reduce((n, j) => n + j.totalBytes, 0);
    return {
      label: jobs.length === 1 ? jobs[0]!.meta.title : plural(jobs.length, 'download'),
      pct: percent(done, total),
      speed: this.speed > 0 ? rate(this.speed) : '',
      items: jobs.map((j) => ({ id: j.id, name: j.meta.title, art: j.meta.art, pct: j.state === 'queued' ? null : percent(j.doneBytes, j.totalBytes) })),
    };
  });
  #rev = -1;
  #waiters = new Map<string, (ok: boolean) => void>();

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
      void api.downloads.history().then((h) => {
        this.history = h ?? [];
        this.#settle();
      });
    }
  }

  /** Resolves once a job has finished: true when it completed, false when it failed or was cancelled. */
  until(id: string): Promise<boolean> {
    if (!id) return Promise.resolve(true);
    return new Promise((resolve) => {
      this.#waiters.set(id, resolve);
      this.#settle();
    });
  }

  #settle() {
    for (const [id, resolve] of this.#waiters) {
      if (this.active.some((j) => j.id === id)) continue;
      const done = this.history.find((j) => j.id === id);
      if (!done) continue;
      this.#waiters.delete(id);
      resolve(done.state === 'done');
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

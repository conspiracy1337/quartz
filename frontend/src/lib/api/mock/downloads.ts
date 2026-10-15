import type { Job, JobMeta, JobStep, StepState, TaskSnapshot } from '../types';

const MB = 1e6;
const MIN = 60_000;

const step = (name: string, unit: JobStep['unit'], done: number, total: number, state: StepState): JobStep => ({ name, unit, done, total, state });
function job(id: string, meta: JobMeta, state: Job['state'], doneBytes: number, totalBytes: number, steps: JobStep[], extra: Partial<Job> = {}): Job {
  return { id, meta, state, doneBytes, totalBytes, speed: 0, etaSec: 0, steps, retryable: false, createdAt: Date.now() - 10 * MIN, finishedAt: 0, ...extra };
}

/** Browser-preview stand-in for Go's job engine: the design's fixtures plus working controls. */
export class MockTasks {
  active: Job[] = [
    job('a00000000001', { title: 'Cobblemon Official Modpack', subtitle: 'Installing as a new instance', art: { background: '#f2a25c', initial: 'C' }, kind: 'install' }, 'running', 44 * MB, 71 * MB, [
      step('Minecraft 1.21.1 and libraries', 'bytes', 31 * MB, 31 * MB, 'done'),
      step('Fabric Loader 0.16.9', 'bytes', 1 * MB, 1 * MB, 'done'),
      step('Mods', 'count', 41, 74, 'run'),
      step('Assets', 'count', 2318, 3604, 'run'),
      step('1 mod needs a manual download', 'count', 0, 1, 'warn'),
    ], { speed: 12.4 * MB, etaSec: 40 }),
    job('a00000000002', { title: 'Fabulously Optimized', subtitle: 'Updating 4 mods', art: { background: '#3ecf8e', initial: 'F' }, kind: 'update' }, 'queued', 0, 6.2 * MB, [
      step('Sodium 0.6.5 to 0.6.6', 'bytes', 0, 2.1 * MB, 'queued'),
      step('Lithium 0.14.3 to 0.14.4', 'bytes', 0, 0.7 * MB, 'queued'),
      step('Iris Shaders 1.8.1 to 1.8.3', 'bytes', 0, 2.4 * MB, 'queued'),
      step('Not Enough Animations 1.9.2 to 1.9.4', 'bytes', 0, 1 * MB, 'queued'),
    ]),
  ];
  history: Job[] = [
    job('f00000000001', { title: 'Iris Shaders 1.8.3', subtitle: 'Fabulously Optimized', art: { background: '#b48cf2', initial: 'I' }, kind: 'update' }, 'done', 2.4 * MB, 2.4 * MB, [], { finishedAt: Date.now() - 2 * MIN }),
    job('f00000000002', { title: 'Complementary Reimagined r5.4', subtitle: 'Fabulously Optimized', art: { background: '#f2b36b', initial: 'C' }, kind: 'install' }, 'done', 1.1 * MB, 1.1 * MB, [], { finishedAt: Date.now() - 3 * 60 * MIN }),
    job('f00000000003', { title: 'Vanilla 1.21.4 game files', subtitle: 'Vanilla 1.21.4', art: { background: '#6b8afd', initial: 'V' }, kind: 'install' }, 'done', 188 * MB, 188 * MB, [], { finishedAt: Date.now() - 20 * 60 * MIN }),
    job('f00000000004', { title: 'Java 21.0.5 runtime', subtitle: 'Eclipse Temurin', art: { background: '#5a5a63', initial: 'J' }, kind: 'java' }, 'failed', 48 * MB, 190 * MB, [], { finishedAt: Date.now() - 5 * MIN, error: 'Connection reset after 3 tries', retryable: true }),
  ];
  rev = 1;

  snapshot(): TaskSnapshot {
    const running = this.active.filter((j) => j.state === 'running');
    const speed = running.reduce((n, j) => n + j.speed, 0);
    return { active: this.active, speed, etaSec: speed ? 60 : 0, historyRev: this.rev };
  }

  #set(id: string, from: Job['state'][], to: Job['state']) {
    const j = this.active.find((x) => x.id === id);
    if (j && from.includes(j.state)) j.state = to;
  }
  pause = (id: string) => this.#set(id, ['queued', 'running'], 'paused');
  resume = (id: string) => this.#set(id, ['paused'], 'running');
  cancel = (id: string) => {
    this.active = this.active.filter((j) => j.id !== id);
  };
  retry = (id: string) => {
    const j = this.history.find((x) => x.id === id && x.retryable);
    if (!j) return;
    this.history = this.history.filter((x) => x !== j);
    this.active.push({ ...j, state: 'queued', error: '', finishedAt: 0 });
    this.rev++;
  };
  pauseAll = () => this.active.forEach((j) => this.pause(j.id));
  resumeAll = () => this.active.forEach((j) => this.resume(j.id));
  cancelAll = () => {
    this.active = [];
  };
  retryAllFailed = () => this.history.filter((j) => j.state === 'failed').forEach((j) => this.retry(j.id));
  clearFinished = () => {
    this.history = this.history.filter((j) => j.state === 'failed');
    this.rev++;
  };
}

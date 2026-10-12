// UI session state that outlives a single screen: the open overlay, game launches,
// local servers, the undo toast and the self-update banner. In M0 the long-running
// parts (launch steps, server start) are simulated; M2 and M9 drive them from Go.

import { api } from './api';
import { errorText } from './format';
import { hasBridge, onEvent } from './bridge';
import { revs } from './revs.svelte';
import { go } from './router.svelte';

export type Overlay =
  | { kind: 'add-account'; mode?: AddAccountMode; accountId?: string }
  | { kind: 'install'; projectId: string }
  | { kind: 'crash'; instanceId: string }
  | { kind: 'blocked' }
  | { kind: 'host'; instanceId: string }
  | { kind: 'palette'; mode: 'commands' | 'instances' }
  | { kind: 'shortcuts' }
  | { kind: 'rename'; instanceId: string }
  | { kind: 'icon'; instanceId: string };

export type AddAccountMode = 'browser' | 'code' | 'offline';

export const LAUNCH_STEPS = ['Signing in', 'Checking mods', 'Java', 'Assets and libraries', 'Sandbox', 'Starting the game'] as const;

/** accountId "" is the default account. */
interface Launch { instanceId: string; accountId: string; stage: number; details?: string[]; pct?: number }

interface LaunchProgress { instanceId: string; stage: number; details: string[]; pct: number }
interface LaunchFailure { instanceId: string; stage: number; message: string; code?: string; accountId?: string }
interface GameState { instanceId: string; runId: string; state: 'running' | 'exited'; exitCode: number; result?: string }
export interface Toast {
  subject: string;
  text: string;
  /** trash: "Moved <subject> <text>" with Undo; info and error: a short notice. */
  tone?: 'trash' | 'info' | 'error';
  undo?: (() => void) | undefined;
  action?: { label: string; run: () => void };
}

const STEP_MS = 700;
const TOAST_MS = 8000;
const HOST_START_MS = 1600;

class Session {
  overlay = $state<Overlay | null>(null);
  launch = $state<Launch | null>(null);
  running = $state<Record<string, boolean>>({});
  hosting = $state<Record<string, 'starting' | 'running'>>({});
  toast = $state<Toast | null>(null);
  /** The right-click "Play as" menu of an instance, at a window position. */
  launchAs = $state<{ instanceId: string; x: number; y: number } | null>(null);
  /** Self-update arrives in M11; until then only the preview shows the design's banner. */
  updateReady = $state(!hasBridge());

  #launchTimer: ReturnType<typeof setInterval> | undefined;
  #toastTimer: ReturnType<typeof setTimeout> | undefined;

  open(o: Overlay) {
    this.overlay = o;
  }

  close() {
    this.overlay = null;
  }

  /** Starts a launch with the default account, or once with another ("Launch as").
      Inside the app Go runs it and reports through events; the browser preview walks
      through the steps on a timer. */
  play(instanceId: string, accountId = '') {
    clearInterval(this.#launchTimer);
    this.overlay = null;
    this.launch = { instanceId, accountId, stage: 0 };
    if (hasBridge()) {
      api.launch.play(instanceId, accountId).catch((e) => {
        this.launch = null;
        this.fail('Play', e);
      });
      return;
    }
    this.#launchTimer = setInterval(() => {
      if (!this.launch) return clearInterval(this.#launchTimer);
      if (this.launch.stage < LAUNCH_STEPS.length - 1) {
        this.launch.stage++;
        return;
      }
      clearInterval(this.#launchTimer);
      this.running[instanceId] = true;
      this.launch = null;
    }, STEP_MS);
  }

  /** Opens an instance's menu (its actions and "Play as") for a right-click, or for Shift+F10 / the Menu key (no pointer position). */
  openLaunchAs(instanceId: string, e: MouseEvent) {
    e.preventDefault();
    let { clientX: x, clientY: y } = e;
    if (!x && !y && e.currentTarget instanceof Element) {
      const r = e.currentTarget.getBoundingClientRect();
      x = r.left + 12;
      y = r.bottom;
    }
    this.launchAs = { instanceId, x, y };
  }

  cancelLaunch() {
    clearInterval(this.#launchTimer);
    if (hasBridge() && this.launch) void api.launch.cancelLaunch(this.launch.instanceId);
    this.launch = null;
  }

  /** Asks the game to quit; a second click forces it. */
  stop(instanceId: string) {
    if (hasBridge()) {
      api.launch.stop(instanceId).catch((e) => this.fail('Stop', e));
      return;
    }
    delete this.running[instanceId];
  }

  /** Reads which games are running (after the window reloads). */
  async load() {
    const s = await api.launch.status();
    this.running = Object.fromEntries((s.running ?? []).map((id) => [id, true]));
  }

  /** Follows Go's launch and game events; returns the unsubscribe function. */
  listen(): () => void {
    const offs = [
      onEvent<LaunchProgress>('launch:progress', (p) => (this.launch = { accountId: this.launch?.instanceId === p.instanceId ? this.launch.accountId : '', instanceId: p.instanceId, stage: p.stage, details: p.details, pct: p.pct })),
      onEvent<LaunchFailure>('launch:failed', (f) => {
        if (this.launch?.instanceId === f.instanceId) this.launch = null;
        if (f.code === 'no-account') this.open({ kind: 'add-account', mode: 'offline' });
        else if (f.code === 'sign-in-expired') this.open({ kind: 'add-account', mode: 'browser', ...(f.accountId ? { accountId: f.accountId } : {}) });
        else if (f.code !== 'cancelled') this.notify({ tone: 'error', subject: "Couldn't start the game", text: f.message });
      }),
      onEvent<GameState>('game:state', (g) => {
        if (g.state === 'running') {
          this.running[g.instanceId] = true;
          if (this.launch?.instanceId === g.instanceId) this.launch = null;
          return;
        }
        delete this.running[g.instanceId];
        revs.bump('instances');
        if (g.result === 'crash') {
          this.notify({
            tone: 'error', subject: 'The game stopped', text: `with exit code ${g.exitCode}`,
            action: { label: 'View log', run: () => go({ name: 'instance', id: g.instanceId, tab: 'logs' }) },
          });
        }
      }),
    ];
    return () => offs.forEach((off) => off());
  }

  startHost(instanceId: string) {
    this.overlay = null;
    this.hosting[instanceId] = 'starting';
    setTimeout(() => {
      if (this.hosting[instanceId]) this.hosting[instanceId] = 'running';
    }, HOST_START_MS);
  }

  stopHost(instanceId: string) {
    delete this.hosting[instanceId];
  }

  notify(t: Toast) {
    clearTimeout(this.#toastTimer);
    this.toast = t;
    this.#toastTimer = setTimeout(() => (this.toast = null), TOAST_MS);
  }

  /** Shows an error from a failed action as a notice. */
  fail(subject: string, err: unknown) {
    this.notify({ tone: 'error', subject, text: errorText(err) });
  }

  dismissToast() {
    clearTimeout(this.#toastTimer);
    this.toast = null;
  }
}

export const session = new Session();

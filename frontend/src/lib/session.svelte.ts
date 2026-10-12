// UI session state that outlives a single screen: the open overlay, game launches,
// local servers, the undo toast and the self-update banner. In M0 the long-running
// parts (launch steps, server start) are simulated; M2 and M9 drive them from Go.

export type Overlay =
  | { kind: 'add-account'; mode?: AddAccountMode }
  | { kind: 'install'; projectId: string }
  | { kind: 'crash'; instanceId: string }
  | { kind: 'blocked' }
  | { kind: 'host'; instanceId: string }
  | { kind: 'palette'; mode: 'commands' | 'instances' }
  | { kind: 'shortcuts' };

export type AddAccountMode = 'browser' | 'code' | 'offline';

export const LAUNCH_STEPS = ['Signing in', 'Checking mods', 'Java', 'Assets and libraries', 'Sandbox', 'Starting the game'] as const;

interface Launch { instanceId: string; stage: number }
interface Toast { subject: string; text: string; undo?: () => void }

const STEP_MS = 700;
const TOAST_MS = 8000;
const HOST_START_MS = 1600;

class Session {
  overlay = $state<Overlay | null>(null);
  launch = $state<Launch | null>(null);
  running = $state<Record<string, boolean>>({});
  hosting = $state<Record<string, 'starting' | 'running'>>({});
  toast = $state<Toast | null>(null);
  updateReady = $state(true);

  #launchTimer: ReturnType<typeof setInterval> | undefined;
  #toastTimer: ReturnType<typeof setTimeout> | undefined;

  open(o: Overlay) {
    this.overlay = o;
  }

  close() {
    this.overlay = null;
  }

  /** Starts a launch and walks through the launch steps. */
  play(instanceId: string) {
    clearInterval(this.#launchTimer);
    this.overlay = null;
    this.launch = { instanceId, stage: 0 };
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

  cancelLaunch() {
    clearInterval(this.#launchTimer);
    this.launch = null;
  }

  stop(instanceId: string) {
    delete this.running[instanceId];
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

  dismissToast() {
    clearTimeout(this.#toastTimer);
    this.toast = null;
  }
}

export const session = new Session();

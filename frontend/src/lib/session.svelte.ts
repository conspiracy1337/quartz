// UI session state that outlives a single screen: the open overlay, game launches,
// the local server, the undo toast and the self-update banner. Go drives launches and
// the server through events; the browser preview walks through them on timers.

import { api } from './api';
import type { HostOptions, HostState } from './api/host';
import type { UpdateState } from './api/system';
import type { ContentKind, InstallPlan, PlayInto, ServerEntry, Src } from './api/types';
import { copyText } from './clipboard';
import { errorText } from './format';
import { rememberCount } from './query.svelte';
import type { TargetPick } from './instances';
import { hasBridge, onEvent } from './bridge';
import { revs } from './revs.svelte';
import { go } from './router.svelte';

export type Overlay =
  | { kind: 'add-account'; mode?: AddAccountMode; accountId?: string }
  /** `plan` is one already made (installInto's), so the dialog doesn't plan again. */
  | { kind: 'install'; ref: string; source: Src; instanceId: string; content: ContentKind; version?: string; plan?: InstallPlan }
  /** A run's crash (runId), or a report in crash-reports (file). */
  | { kind: 'crash'; instanceId: string; runId?: string; file?: string }
  /** Add a server, or edit `entry`. */
  | { kind: 'server'; instanceId: string; entry?: ServerEntry }
  | { kind: 'copy-world'; instanceId: string; folder: string; name: string; sub: string }
  | { kind: 'rename-world'; instanceId: string; folder: string; name: string }
  | { kind: 'blocked'; jobId: string }
  | { kind: 'host'; instanceId: string }
  | { kind: 'palette'; mode: 'commands' | 'instances' }
  | { kind: 'shortcuts' }
  | { kind: 'rename'; instanceId: string }
  | { kind: 'look'; instanceId: string }
  | { kind: 'shortcut'; instanceId: string }
  | { kind: 'export'; instanceId: string };

export type AddAccountMode = 'browser' | 'code' | 'offline';

export const LAUNCH_STEPS = ['Signing in', 'Checking mods', 'Java', 'Assets and libraries', 'Sandbox', 'Starting the game'] as const;

/** accountId "" is the default account. */
interface Launch { instanceId: string; accountId: string; stage: number; details?: string[]; pct?: number }

interface LaunchProgress { instanceId: string; stage: number; details: string[]; pct: number }
interface LaunchFailure { instanceId: string; stage: number; message: string; code?: string; accountId?: string }
/** copies: how many copies of the instance run after this change; kept: worlds this run
    changed while another copy did too, kept beside them as "W (copy 2)". */
interface GameState { instanceId: string; runId: string; state: 'running' | 'exited'; exitCode: number; result?: string; copies: number; kept?: { world: string; as: string }[] }
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
const TOAST_QUEUE = 3;
const HOST_START_MS = 1600;

class Session {
  overlay = $state<Overlay | null>(null);
  launch = $state<Launch | null>(null);
  running = $state<Record<string, boolean>>({});
  /** The one local server, while it starts or runs. */
  hosting = $state<HostState | null>(null);
  /** Whether its panel shows (the header's address chip toggles it). */
  hostPanel = $state(true);
  /** The panel's server after it stopped (see hostShown). */
  hostStopped = $state<HostState | null>(null);
  toast = $state<Toast | null>(null);
  /** An instance's menu (right-click, or the header's ⋮), at a window position. */
  instanceMenu = $state<{ instanceId: string; x: number; y: number } | null>(null);
  /** The bottom bar's download queue is open. */
  downloadTray = $state(true);
  /** An install started from Browse or a project page shows its queue in a floating panel until it ends or is closed. */
  downloadPanel = $state(false);
  /** The instance Browse and project pages install into, once picked (else the most recent). */
  browseTarget = $state<TargetPick | null>(null);
  /** Picks the instance Browse and project pages add to. */
  addTo(id: string) {
    this.browseTarget = { id, at: Date.now() };
  }
  /** What Browse or a project page already shows of the modpack it sends to Import, so its name,
      picture and summary appear at once while Go reads the pack. */
  packHint = $state<{ ref: string; name: string; author: string; summary: string; icon?: string } | null>(null);
  /** A pack version the Import screen sends to an instance's update page, picked there instead of the offered one. */
  packPick = $state<{ instanceId: string; version: string } | null>(null);
  /** Items a content tab's selection sends to the update review, ticked there instead of its defaults. */
  updatePick = $state<Set<string> | null>(null);
  /** Browse's "Hide projects with" choices, kept while the launcher runs. */
  browseHide = $state({ ai: true, ads: false, installed: true });
  /** A file a launch saved, for the Log files viewer to show (set by Launch history's Open). */
  runFile = $state<{ instanceId: string; runId: string; kind: 'log' | 'cmd'; title: string } | null>(null);
  /** The launcher updater's state (Go's update:state). */
  update = $state<UpdateState | null>(null);
  /** The update state has answered (or failed to); until then App holds the banner's place if it showed last run. */
  updateKnown = $state(false);
  /** "Later" hides the banner until the next start; the update installs when the launcher quits. */
  updateLater = $state(false);
  get updateReady() {
    return this.update?.status === 'ready' && !this.updateLater;
  }

  #launchTimer: ReturnType<typeof setInterval> | undefined;
  #toastTimer: ReturnType<typeof setTimeout> | undefined;
  #toastQueue: Toast[] = [];
  #toastLeft = TOAST_MS;
  #toastStarted = 0;
  #toastPaused = false;
  /** Bumps for every toast shown, so its draining bar starts over. */
  toastSeq = $state(0);

  open(o: Overlay) {
    this.overlay = o;
  }

  close() {
    this.overlay = null;
  }

  /** Starts a launch with the default account, or once with another ("Launch as").
      Inside the app Go runs it and reports through events; the browser preview walks
      through the steps on a timer. */
  play(instanceId: string, accountId = '', into?: PlayInto) {
    clearInterval(this.#launchTimer);
    this.overlay = null;
    this.launch = { instanceId, accountId, stage: 0 };
    if (hasBridge()) {
      (into ? api.launch.playInto(instanceId, accountId, into) : api.launch.play(instanceId, accountId)).catch((e) => {
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
  openInstanceMenu(instanceId: string, e: MouseEvent) {
    e.preventDefault();
    let { clientX: x, clientY: y } = e;
    if (!x && !y && e.currentTarget instanceof Element) {
      const r = e.currentTarget.getBoundingClientRect();
      x = r.left + 12;
      y = r.bottom;
    }
    this.instanceMenu = { instanceId, x, y };
  }

  cancelLaunch() {
    clearInterval(this.#launchTimer);
    if (hasBridge() && this.launch) void api.launch.cancelLaunch(this.launch.instanceId);
    this.launch = null;
  }

  /** Whether an instance is starting or running: its Play button shows Stop. */
  busy(instanceId: string) {
    return !!this.running[instanceId] || this.launch?.instanceId === instanceId;
  }

  /** Asks the game to quit (a second click forces it), or cancels a launch still preparing. */
  stop(instanceId: string) {
    if (!this.running[instanceId] && this.launch?.instanceId === instanceId) {
      this.cancelLaunch();
      return;
    }
    if (hasBridge()) {
      api.launch.stop(instanceId).catch((e) => this.fail('Stop', e));
      return;
    }
    delete this.running[instanceId];
  }

  /** A ready update stays ready on the next start, so the banner's place is kept for it then. */
  #setUpdate(u: UpdateState) {
    this.update = u;
    rememberCount('update.banner', u.status === 'ready' ? 1 : 0);
  }

  /** Reads which games are running (after the window reloads). */
  async load() {
    // The update state is applied on its own, so the banner's held place is filled the moment it answers.
    const update = api.updates.state().then((u) => this.#setUpdate(u)).finally(() => (this.updateKnown = true));
    const [s, hosts] = await Promise.all([api.launch.status(), api.host.status(), update]);
    this.running = Object.fromEntries((s.running ?? []).map((id) => [id, true]));
    this.hosting = hosts[0] ?? null;
  }

  /** Follows Go's launch and game events; returns the unsubscribe function. */
  listen(): () => void {
    const offs = [
      onEvent<LaunchProgress>('launch:progress', (p) => (this.launch = { accountId: this.launch?.instanceId === p.instanceId ? this.launch.accountId : '', instanceId: p.instanceId, stage: p.stage, details: p.details, pct: p.pct })),
      onEvent<LaunchFailure>('launch:failed', (f) => {
        if (this.launch?.instanceId === f.instanceId) this.launch = null;
        if (f.code === 'no-account') this.open({ kind: 'add-account', mode: 'offline' });
        else if (f.code === 'sign-in-expired') this.open({ kind: 'add-account', mode: 'browser', ...(f.accountId ? { accountId: f.accountId } : {}) });
        // Java 24+ in the Windows sandbox needs one approval; playing again asks for it.
        else if (f.code === 'sandbox-approval') this.notify({ tone: 'error', subject: "Couldn't start the game", text: f.message, action: { label: 'Approve', run: () => this.play(f.instanceId) } });
        else if (f.code !== 'cancelled') this.notify({ tone: 'error', subject: "Couldn't start the game", text: errorText(f.message) });
      }),
      onEvent<GameState>('game:state', (g) => {
        if (g.state === 'running') {
          this.running[g.instanceId] = true;
          if (this.launch?.instanceId === g.instanceId) this.launch = null;
          return;
        }
        if (!g.copies) delete this.running[g.instanceId];
        revs.bump('instances');
        for (const k of g.kept ?? []) this.notify({ subject: k.world, text: `changed in both copies, kept as ${k.as}` });
        // A crash (or a game that never started) opens the crash dialog with its likely cause.
        if (g.result === 'crash' || g.result === 'failed') this.open({ kind: 'crash', instanceId: g.instanceId, runId: g.runId });
      }),
      onEvent<HostState>('host:state', (h) => this.hostChanged(h)),
      onEvent<UpdateState>('update:state', (u) => this.#setUpdate(u)),
    ];
    return () => offs.forEach((off) => off());
  }

  /** The server the panel shows: the running one, else the one that just stopped, which
      stays (with Start) until the panel is closed. */
  get hostShown(): HostState | null {
    return this.hosting ?? this.hostStopped;
  }

  /** Closes the server panel; a stopped server's panel goes for good. */
  closeHostPanel() {
    this.hostPanel = false;
    this.hostStopped = null;
  }

  /** Starts the panel's stopped server again with the options it last used. */
  async restartHost(instanceId: string) {
    try {
      this.startHost(instanceId, (await api.host.setup(instanceId)).options);
    } catch (e) {
      this.fail("Couldn't start the server", e);
    }
  }

  /** Starts hosting an instance. Inside the app Go reports on host:state; the preview
      goes from starting to running on a timer. */
  startHost(instanceId: string, o: HostOptions) {
    this.overlay = null;
    this.hostPanel = true;
    this.hostStopped = null;
    if (hasBridge()) {
      api.host.start(instanceId, o).catch((e) => this.fail("Couldn't start the server", e));
      return;
    }
    const base: HostState = { instanceId, runId: 'preview', state: 'starting', stage: 'Backing up Northwatch, preparing spawn area', port: o.port, players: 0, max: o.maxPlayers,
      address: `${o.lan ? '192.168.1.24' : 'localhost'}:${o.port}`, ...(o.lan ? { lan: '192.168.1.24' } : {}) };
    this.hosting = base;
    setTimeout(() => {
      if (this.hosting?.runId === 'preview') this.hosting = { ...base, state: 'running', players: 1 };
    }, HOST_START_MS);
  }

  /** Asks the server to save and stop; a second click forces it. */
  stopHost(instanceId: string) {
    if (hasBridge()) {
      api.host.stop(instanceId).catch((e) => this.fail('Stop', e));
      return;
    }
    if (this.hosting) this.hostStopped = { ...this.hosting, state: 'stopped', result: 'ok', players: 0 };
    this.hosting = null;
  }

  /** Follows the server's state; a server that ends badly says so. */
  hostChanged(h: HostState) {
    if (h.state !== 'stopped') {
      this.hosting = h;
      return;
    }
    if (this.hosting?.runId === h.runId) {
      this.hosting = null;
      this.hostStopped = h;
    }
    revs.bump('worlds');
    if (h.result === 'failed') this.notify({ tone: 'error', subject: "Couldn't start the server", text: h.message ?? '' });
    else if (h.result === 'crash') {
      this.notify({ tone: 'error', subject: 'The server', text: 'stopped unexpectedly', action: { label: 'Console', run: () => go({ name: 'instance', id: h.instanceId, tab: 'logs' }) } });
    }
  }

  /** Shows a toast. One carrying Undo or an action is never replaced: later toasts
      wait their turn (at most TOAST_QUEUE). A plain notice gives way at once. */
  notify(t: Toast) {
    const same = (o: Toast | null) => !!o && o.subject === t.subject && o.text === t.text;
    if (same(this.toast) || this.#toastQueue.some(same)) return;
    const cur = this.toast;
    if (cur && (cur.undo || cur.action)) {
      this.#toastQueue = [...this.#toastQueue, t].slice(-TOAST_QUEUE);
      return;
    }
    this.#show(t);
  }

  #show(t: Toast) {
    clearTimeout(this.#toastTimer);
    this.toast = t;
    this.toastSeq++;
    this.#toastLeft = TOAST_MS;
    this.#toastPaused = false;
    this.#runToast();
  }

  #runToast() {
    this.#toastStarted = Date.now();
    this.#toastTimer = setTimeout(() => this.dismissToast(), this.#toastLeft);
  }

  /** Hover or focus holds the toast (and its draining bar) until it is left. */
  holdToast(hold: boolean) {
    if (!this.toast || hold === this.#toastPaused) return;
    this.#toastPaused = hold;
    if (hold) {
      clearTimeout(this.#toastTimer);
      this.#toastLeft = Math.max(0, this.#toastLeft - (Date.now() - this.#toastStarted));
    } else {
      this.#runToast();
    }
  }

  /** Uploads to mclo.gs, copies the link and offers to open it. */
  async upload(fn: () => Promise<string>) {
    try {
      const url = await fn();
      const copied = await copyText(url);
      this.notify({ subject: 'Uploaded to mclo.gs', text: copied ? 'The link is copied.' : url, action: { label: 'Open', run: () => void api.app.openUrl(url) } });
    } catch (e) {
      this.fail('Upload', e);
    }
  }

  /** Copies text and says so. */
  async copy(text: string, what: string) {
    if (await copyText(text)) this.notify({ subject: what, text: 'copied' });
    else this.notify({ tone: 'error', subject: "Couldn't copy", text: 'The system refused the clipboard.' });
  }

  /** Shows an error from a failed action as a notice. */
  fail(subject: string, err: unknown) {
    this.notify({ tone: 'error', subject, text: errorText(err) });
  }

  dismissToast() {
    clearTimeout(this.#toastTimer);
    const [next, ...rest] = this.#toastQueue;
    this.#toastQueue = rest;
    if (next) this.#show(next);
    else this.toast = null;
  }
}

export const session = new Session();

import { api } from './api';
import type { Art, ContentItem, MergedServer, ServerEntry, ServerStats } from './api/types';
import { prefs } from './prefs.svelte';
import { revs } from './revs.svelte';
import { session } from './session.svelte';

/** A row of the servers table (ServerTable): one from an instance's list, or Home's merged one. */
export interface ServerRow {
  id: string;
  name: string;
  address: string;
  art: Art;
  stats: ServerStats | undefined;
  /** Instances Join can use, the default (the one last used on it) first. */
  instances: string[];
  packs?: ServerEntry['packs'] | undefined;
}

/** What the servers table does where it shows: an instance's Servers tab, or Home. */
export interface ServerScope {
  /** Names the sort and Online filter the table keeps while the launcher runs. */
  key: string;
  /** Join buttons (with ▾ for another instance) instead of Join in the row's ⋮. */
  buttons: boolean;
  /** Select boxes and bulk Remove. */
  bulk: boolean;
  join(r: ServerRow, instanceId?: string): void;
  /** Pings every server again. */
  refresh(): Promise<unknown>;
  /** Takes rows out; resolves with what brings them back, or null when nothing went. */
  remove(rows: ServerRow[]): Promise<(() => Promise<unknown>) | null>;
  edit(r: ServerRow): void;
  /** Moves r up (by < 0) or down; rows is the whole list in its own order. */
  move(r: ServerRow, by: number, rows: ServerRow[]): Promise<unknown>;
}

/** How often the server tables on show ping their servers again. */
const REPING_MS = 60_000;

/**
 * One clock for every servers table, so Home and an instance's Servers tab are pinged on the
 * same beat and show the same answers (Go shares each address's newest answer between them).
 * It runs while a table shows and keeps its beat across page changes: a table that mounts
 * shows Go's cached answers and is pinged again on the next beat.
 */
class PingClock {
  #scopes = new Set<ServerScope>();
  #timer: ReturnType<typeof setTimeout> | undefined;
  #last = 0;

  /** Pings scope on every beat until the returned function is called. */
  join(scope: ServerScope): () => void {
    this.#scopes.add(scope);
    if (!this.#last) this.#last = Date.now();
    if (!this.#timer) this.#arm();
    return () => {
      this.#scopes.delete(scope);
      if (this.#scopes.size) return;
      clearTimeout(this.#timer);
      this.#timer = undefined;
    };
  }

  #arm() {
    this.#timer = setTimeout(() => this.#tick(), Math.max(0, this.#last + REPING_MS - Date.now()));
  }

  #tick() {
    this.#last = Date.now();
    for (const s of this.#scopes) void s.refresh().catch(() => {});
    this.#arm();
  }
}
export const pingClock = new PingClock();

/** The Online filter of each table (by its scope's key): on shows only servers that answered. */
export const onlineOnly = $state<Partial<Record<string, boolean>>>({});

const joinWith = (instanceId: string | undefined, address: string) => {
  if (instanceId) session.play(instanceId, '', { kind: 'server', target: address });
};

export const instanceRow = (c: ContentItem, instanceId: string): ServerRow => ({
  id: c.id, name: c.name, address: c.sub, art: c.art, stats: c.server, instances: [instanceId],
});

export const homeRow = (s: MergedServer): ServerRow => ({
  id: s.id, name: s.name, address: s.address, art: s.art, stats: s, instances: s.instances, packs: s.packs,
});

/** An instance's own servers.dat: edits name each row by its place and address. */
export function instanceScope(instanceId: string): ServerScope {
  const entry = (r: ServerRow) => api.servers.entry(instanceId, r.id);
  return {
    key: 'instance',
    buttons: false,
    bulk: true,
    join: (r) => joinWith(instanceId, r.address),
    refresh: async () => {
      await api.servers.list(instanceId, true);
      revs.bump('servers');
    },
    // In one call, so one Undo brings them all back.
    remove: async (rows) => {
      const refs = rows.flatMap((r) => {
        const e = entry(r);
        return e ? [{ index: e.index, address: e.address }] : [];
      });
      if (!refs.length) return null;
      const token = await api.servers.remove(instanceId, refs);
      revs.bump('servers');
      return () => api.servers.undo(instanceId, token).then(() => revs.bump('servers'));
    },
    edit: (r) => {
      const e = entry(r);
      session.open({ kind: 'server', instanceId, ...(e ? { entry: e } : {}) });
    },
    move: async (r, by) => {
      const e = entry(r);
      if (!e) return;
      await api.servers.move(instanceId, e.index, e.address, by);
      revs.bump('servers');
    },
  };
}

/**
 * Home's rows in the player's order (Move up and down on Home); servers it doesn't
 * name yet keep Go's order (online first, then last used) after the ones it does.
 */
export function inHomeOrder(rows: ServerRow[], order: string[] | undefined): ServerRow[] {
  if (!order?.length) return rows;
  const at = new Map(order.map((id, i) => [id, i]));
  return rows
    .map((r, i) => ({ r, k: at.get(r.id) ?? order.length + i }))
    .sort((a, b) => a.k - b.k)
    .map((x) => x.r);
}

/** Home's order follows a row whose ID changed (an edit gave it another address). */
export function renameInHomeOrder(from: string, to: string) {
  if (from === to || !prefs.value?.general.serverOrder?.includes(from)) return;
  void prefs.update((s) => {
    const ids = (s.general.serverOrder ?? []).map((id) => (id === from ? to : id));
    s.general.serverOrder = ids.filter((id, i) => ids.indexOf(id) === i);
  });
}

/**
 * Home's servers, merged from every instance. Edit and Remove change the server in
 * every instance that lists it, so the row stays one server; the order is Home's own.
 */
export const homeScope: ServerScope = {
  key: 'home',
  buttons: true,
  bulk: false,
  join: (r, instanceId = r.instances[0]) => joinWith(instanceId, r.address),
  refresh: async () => {
    await api.home.servers(true);
    revs.bump('servers');
  },
  remove: async (rows) => {
    const parts = await api.home.removeServers(rows.map((r) => r.id));
    if (!parts.length) return null;
    revs.bump('servers');
    return () => Promise.all(parts.map((p) => api.servers.undo(p.instance, p.token))).then(() => revs.bump('servers'));
  },
  edit: (r) => {
    const entry = { index: -1, name: r.name, address: r.address, packs: r.packs ?? 'prompt' };
    session.open({ kind: 'server', instanceId: r.instances[0] ?? '', entry, home: r.id });
  },
  move: async (r, by, rows) => {
    const ids = rows.map((x) => x.id);
    const at = ids.indexOf(r.id);
    if (at < 0 || at + by < 0 || at + by >= ids.length) return;
    ids.splice(at, 1);
    ids.splice(at + by, 0, r.id);
    await prefs.update((s) => (s.general.serverOrder = ids));
  },
};

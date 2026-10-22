import { api } from './api';
import type { Art, ContentItem, MergedServer, ServerStats } from './api/types';
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
}

/** What the servers table's actions do where it shows: an instance's Servers tab, or Home. */
export interface ServerScope {
  /** Names the sort and Online toggle the table keeps while the launcher runs. */
  key: string;
  /** Join has a ▾ to join with another instance. */
  pick: boolean;
  join(r: ServerRow, instanceId?: string): void;
  /** Pings every server again. */
  refresh(): Promise<unknown>;
  /** Takes rows out; resolves with what brings them back, or null when nothing went. */
  remove(rows: ServerRow[]): Promise<(() => Promise<unknown>) | null>;
  edit?: (r: ServerRow) => void;
  move?: (r: ServerRow, by: number) => Promise<unknown>;
}

const joinWith = (instanceId: string | undefined, address: string) => {
  if (instanceId) session.play(instanceId, '', { kind: 'server', target: address });
};

export const instanceRow = (c: ContentItem, instanceId: string): ServerRow => ({
  id: c.id, name: c.name, address: c.sub, art: c.art, stats: c.server, instances: [instanceId],
});

export const homeRow = (s: MergedServer): ServerRow => ({
  id: s.id, name: s.name, address: s.address, art: s.art, stats: s, instances: s.instances,
});

/** An instance's own servers.dat: edits name each row by its place and address. */
export function instanceScope(instanceId: string): ServerScope {
  const entry = (r: ServerRow) => api.servers.entry(instanceId, r.id);
  return {
    key: 'instance',
    pick: false,
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
 * Home's servers, merged from every instance. A row may be in several instances, so
 * there is no Edit or Move; Remove takes it out of all of them.
 */
export const homeScope: ServerScope = {
  key: 'home',
  pick: true,
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
};

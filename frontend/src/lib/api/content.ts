// Mods, resource packs, shaders and data packs: Go's content service. The browser
// preview serves the design's fixtures and keeps its edits in memory.
import { bridged } from '../bridge';
import { artOf } from '../look';
import { revs } from '../revs.svelte';
import * as Go from '../wailsjs/go/api/Content';
import { content as fixtures } from './mock/content';
import { serversApi, worldsApi } from './game';
import * as inst from './mock/instances';
import * as misc from './mock/misc';
import type {
  AddResult, Art, Compat, ContentChange, ContentInfo, ContentItem, ContentKind, ContentStatus, ModUpdate,
  PendingUpdate, Source, StoreUsage, UpdateReview, ChangelogVersion,
} from './types';

/** The kinds the content service manages; worlds and servers have their own services. */
export type ManagedKind = Exclude<ContentKind, 'worlds' | 'servers'>;
export const managed = (k: ContentKind): k is ManagedKind => k !== 'worlds' && k !== 'servers';

const ICON = /^[0-9a-f]{64}$/;

/** An item's picture: its own icon (served by Go, named by its hash), else a tint and an initial. */
export const contentArt = (name: string, icon?: string): Art => artOf(name, icon && ICON.test(icon) ? `/tex/content/${icon}.png` : undefined);

type GoRow = Awaited<ReturnType<typeof Go.List>>[number];

function toStatus(s: GoRow['status']): ContentStatus | undefined {
  switch (s?.kind) {
    case 'update': return { kind: 'update', version: s.version ?? '' };
    case 'locked': return { kind: 'locked' };
    case 'unknown': return { kind: 'unknown' };
    case 'outdated': return { kind: 'outdated', label: s.label ?? '' };
    case 'world': return { kind: 'world', world: s.world ?? '' };
    case 'problem': return { kind: 'problem', label: s.label ?? '', tone: (s.tone ?? 'warn') as 'warn' | 'danger' | 'info' };
  }
  return undefined;
}

export function toItem(r: GoRow): ContentItem {
  const status = toStatus(r.status);
  return {
    id: r.id, name: r.name, sub: r.sub, meta: r.version, art: contentArt(r.name, r.icon), source: r.source as Source, enabled: r.enabled,
    ...(r.locked ? { locked: true } : {}), ...(status ? { status } : {}), ...(r.group ? { group: r.group } : {}), ...(r.project ? { project: r.project } : {}), ...(r.ref ? { ref: r.ref } : {}), ...(r.clientOnly ? { clientOnly: true } : {}),
  };
}

type GoReview = Awaited<ReturnType<typeof Go.CheckUpdates>>;
const toReview = (r: GoReview): UpdateReview => ({
  checkId: r.checkId,
  updates: (r.updates ?? []).map((u) => ({
    id: u.id, name: u.name, note: u.note, from: u.from, to: u.to, art: contentArt(u.name, u.icon), size: u.size, versions: [],
    ...(u.blocked ? { blocked: true } : {}), ...(u.dependency ? { dependency: true } : {}), ...(u.versions ? { versionsUrl: u.versions } : {}), ...(u.ref ? { ref: u.ref } : {}),
  })),
  locked: r.locked ?? [],
  failed: r.failed ?? [],
});

/** The preview's content: the design's fixtures plus whatever the screens change. */
class MockContent {
  items = structuredClone(fixtures);
  removed = new Map<string, { kind: ContentKind; items: ContentItem[] }>();

  find = (kind: ContentKind, id: string) => this.items[kind].find((c) => c.id === id);
  // Preview items open their project page by their own id.
  list = (_id: string, kind: ContentKind) => this.items[kind].map((c) => ({ ...c, ref: c.ref ?? c.id }));
  info = (_id: string): ContentInfo => ({ shaderMod: 'Iris 1.8.1', worlds: ['Creative Plots', 'Northwatch'] });
  setEnabled = (_id: string, kind: ContentKind, item: string, on: boolean) => {
    const c = this.find(kind, item);
    if (c) c.enabled = on;
  };
  setActiveShader = (_id: string, item: string) => {
    for (const c of this.items.shaders) c.enabled = c.id === item;
  };
  setPackOrder = (_id: string, order: string[]) => {
    const packs = this.items.resourcepacks;
    const on = order.map((id) => packs.find((c) => c.id === id)).filter((c) => c !== undefined);
    on.forEach((c) => (c.enabled = true));
    const off = packs.filter((c) => !order.includes(c.id)).map((c) => ({ ...c, enabled: false }));
    this.items.resourcepacks = [...on, ...off];
  };
  setLocked = (_id: string, item: string, locked: boolean) => {
    const c = this.find('mods', item);
    if (!c) return;
    c.locked = locked;
    if (locked) c.status = { kind: 'locked' };
    else if (c.status?.kind === 'locked') delete c.status;
  };
  remove = (_id: string, kind: ContentKind, ids: string[]) => {
    const token = Math.random().toString(16).slice(2, 18).padEnd(16, '0');
    this.removed.set(token, { kind, items: this.items[kind].filter((c) => ids.includes(c.id)) });
    this.items[kind] = this.items[kind].filter((c) => !ids.includes(c.id));
    return token;
  };
  undo = (_id: string, token: string) => {
    const r = this.removed.get(token);
    if (r) this.items[r.kind].push(...r.items);
    this.removed.delete(token);
  };
  check = (_id: string, kind: ContentKind): UpdateReview => ({
    checkId: 'preview', updates: kind === 'mods' ? misc.modUpdates : [],
    locked: kind === 'mods' ? [{ id: '7', name: 'Distant Horizons', version: '2.3.0-b' }] : [], failed: [],
  });
  changelog = (_check: string, id: string): ChangelogVersion[] => misc.modUpdates.find((u) => u.id === id)?.versions ?? [];
}

const mock = new MockContent();

const list = bridged((id: string, kind: ContentKind) => Go.List(id, kind).then((rows) => rows.map(toItem)), mock.list);
const check = bridged((id: string, kind: ContentKind) => Go.CheckUpdates(id, kind).then(toReview), mock.check);

/** The last check per instance and kind, so the review screen opens on it without checking twice. */
const reviews = new Map<string, { at: number; review: UpdateReview }>();
const REVIEW_TTL = 5 * 60_000;

/** Runs an edit, then refreshes every content query (Go also says so through content:changed). */
const edit = <A extends unknown[], R>(fn: (...a: A) => Promise<R>) => async (...a: A): Promise<R> => {
  const r = await fn(...a);
  revs.bump('content');
  return r;
};

export const contentApi = {
  list: (id: string, kind: ContentKind): Promise<ContentItem[]> =>
    kind === 'worlds' ? worldsApi.list(id) : kind === 'servers' ? serversApi.list(id) : revs.load('content', () => list(id, kind)),
  info: (id: string) => revs.load('content', bridged(() => Go.Info(id), () => mock.info(id))),
  setEnabled: edit(bridged(Go.SetEnabled, mock.setEnabled)),
  setActiveShader: edit(bridged(Go.SetActiveShader, mock.setActiveShader)),
  setPackOrder: edit(bridged(Go.SetPackOrder, mock.setPackOrder)),
  setLocked: edit(bridged(Go.SetLocked, mock.setLocked)),
  /** Moves items aside; resolves with the token Undo takes. */
  remove: edit(bridged(Go.Remove, mock.remove)),
  undo: edit(bridged(Go.Undo, mock.undo)),
  showInFolder: bridged(Go.ShowInFolder, (_id: string, _item: string) => undefined),
  /** Adds the files of a drop (Go keeps the paths; the UI only ever has the drop id). */
  addDropped: edit(bridged(Go.AddDropped, (_drop: string, _id: string, _kind: ContentKind, _world: string): AddResult => ({ added: [], rejected: [] }))),
  /** Checks for updates; `fresh` skips the last check of the past few minutes. */
  review: async (id: string, kind: ContentKind, fresh = false): Promise<UpdateReview> => {
    const key = `${id}/${kind}`;
    const hit = reviews.get(key);
    if (!fresh && hit && Date.now() - hit.at < REVIEW_TTL) return hit.review;
    const review = await check(id, kind);
    reviews.set(key, { at: Date.now(), review });
    revs.bump('content');
    return review;
  },
  /** Forgets a kind's last check (after applying it). */
  forget: (id: string, kind: ContentKind) => reviews.delete(`${id}/${kind}`),
  changelog: bridged(Go.Changelog, mock.changelog),
  /** Starts the update job; resolves with its id. */
  apply: bridged(Go.ApplyUpdates, (_check: string, _ids: string[]) => ''),
  compatibility: bridged(
    (id: string, mc: string, loader: string) => Go.Compatibility(id, mc, loader),
    (_id: string, mc: string, _loader: string): Compat | null => (mc.startsWith('1.21') ? { matching: 41, total: 48 } : null),
  ),
  pending: (id: string) =>
    revs.load('content', bridged(
      () => Go.Pending(id).then((l): PendingUpdate[] => l.map((u) => ({ name: u.name, art: contentArt(u.name, u.icon), from: u.from, to: u.to }))),
      () => inst.pendingUpdates,
    )),
  changes: bridged((id: string) => Go.Activity(id) as Promise<ContentChange[]>, (_id: string): ContentChange[] => []),
  missing: bridged(Go.Missing, () => [] as { instanceId: string; mod: string; dep: string; more?: number }[]),
  /** Follows the instance a screen shows ("" stops), so outside changes appear. */
  watch: bridged(Go.Watch, (_id: string) => undefined),
  usage: () => revs.load('content', bridged(Go.Usage, (): StoreUsage => ({ blobs: 412, bytes: 3_100_000_000, unused: 18, unusedBytes: 240_000_000 }))),
  cleanUp: bridged(Go.CleanUp, (): StoreUsage => ({ blobs: 394, bytes: 2_860_000_000, unused: 0, unusedBytes: 0 })),
};

export type { ModUpdate };

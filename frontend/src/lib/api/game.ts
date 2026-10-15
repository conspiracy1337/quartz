// Worlds, servers, screenshots, game logs, crashes and Home's widgets: Go's
// services shaped into the types the ported screens already use. The browser
// preview keeps serving the design's fixtures.
import { bridged, hasBridge, preview } from '../bridge';
import { ago, bytes, count } from '../format';
import { artOf, tintOf } from '../look';
import { revs } from '../revs.svelte';
import * as HomeGo from '../wailsjs/go/api/Home';
import * as LogsGo from '../wailsjs/go/api/Logs';
import * as ScreenshotsGo from '../wailsjs/go/api/Screenshots';
import * as ServersGo from '../wailsjs/go/api/Servers';
import * as WorldsGo from '../wailsjs/go/api/Worlds';
import type { servers } from '../wailsjs/go/models';
import { content as fixtures } from './mock/content';
import * as home from './mock/home';
import * as logs from './mock/logs';
import type { Art, ContentItem, CrashInfo, DayPlay, DiskSpace, LogFile, MergedServer, NewsItem, Screenshot, ServerEntry } from './types';

type GoWorld = Awaited<ReturnType<typeof WorldsGo.List>>[number];
type GoServer = Awaited<ReturnType<typeof ServersGo.List>>[number];
type GoMerged = Awaited<ReturnType<typeof HomeGo.Servers>>[number];
type GoShot = Awaited<ReturnType<typeof HomeGo.RecentShots>>[number];

/** A world without its own icon.png: a sky over land, tinted by its name (the design's world tiles). */
const worldArt = (w: GoWorld): Art =>
  w.icon ? artOf(w.name, w.icon) : { background: `linear-gradient(175deg,#7fb7d9 0 45%,${tintOf(w.name)} 45% 72%,#2c5236 72%)` };

/** A server without a picture: its tint fading to the panel colour, as the design draws them. */
const serverArt = (name: string, online: boolean, icon?: string): Art =>
  icon ? artOf(name, icon) : { background: online ? `linear-gradient(160deg,${tintOf(name)},#1a1a1e)` : 'linear-gradient(160deg,#3a3a40,#1a1a1e)' };

export const worldItem = (w: GoWorld): ContentItem => ({
  id: w.folder, name: w.name, sub: `${capital(ago(w.lastPlayed))}, ${bytes(w.bytes)}`, meta: w.mode, art: worldArt(w),
  enabled: true, action: 'Play', group: w.mode,
});

const capital = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

/** "smp.kestrel.gg, 6 of 20 online", "Needs 1.20.1" or "Can not connect" (the design's wording). */
function serverSub(s: GoServer): string {
  if (!s.online) return 'Can not connect';
  if (s.needs) return `Needs ${s.needs}`;
  return `${s.address}, ${s.max ? `${count(s.players)} of ${count(s.max)}` : count(s.players)} online`;
}

export const serverItem = (s: GoServer): ContentItem => ({
  id: String(s.index), name: s.name, sub: serverSub(s), meta: s.online ? `${s.pingMs ?? 0} ms` : 'Offline', art: serverArt(s.name, s.online, s.icon),
  enabled: true, online: s.online, ...(s.online ? { action: 'Join' as const } : { status: { kind: 'offline' as const } }),
});

/** What the server dialog edits, kept beside the rows by their id. */
const entries = new Map<string, ServerEntry>();

const toMerged = (m: GoMerged): MergedServer => ({
  id: m.id, name: m.name, address: m.address, version: m.online ? (m.version ?? '') : 'Offline', online: m.online,
  ...(m.online ? { players: { online: m.players, ...(m.max ? { max: m.max } : {}) }, ...(m.pingMs ? { pingMs: m.pingMs } : {}) } : {}),
  art: serverArt(m.name, m.online, m.icon), instances: m.instances,
});

export const toShot = (s: GoShot): Screenshot => ({
  id: s.id, instanceId: s.instanceId, world: s.world, title: s.title, takenAt: s.takenAt, file: s.file,
  width: s.width, height: s.height, bytes: s.bytes, art: artOf(s.title, s.thumb), image: s.image,
});

const worldsList = (id: string) => WorldsGo.List(id).then((l) => l.map(worldItem));
const serversList = (id: string, force: boolean) =>
  ServersGo.List(id, force).then((l) =>
    l.map((s) => {
      entries.set(`${id}/${s.index}`, { index: s.index, name: s.name, address: s.address, packs: s.packs as ServerEntry['packs'] });
      return serverItem(s);
    }));

export const worldsApi = {
  list: (id: string): Promise<ContentItem[]> => (hasBridge() ? revs.load('worlds', () => worldsList(id)) : preview(fixtures.worlds)),
  import: bridged(WorldsGo.Import, (_id: string) => ''),
  importDropped: bridged(WorldsGo.ImportDropped, (_drop: string, _id: string): string[] => []),
  export: bridged(WorldsGo.Export, (_id: string, _folder: string) => false),
  copyTo: bridged(WorldsGo.CopyTo, (_id: string, _folder: string, _target: string) => ''),
  rename: bridged(WorldsGo.Rename, (_id: string, _folder: string, _name: string) => undefined),
  remove: bridged(WorldsGo.Delete, (_id: string, _folder: string) => '0123456789abcdef'),
  undo: bridged(WorldsGo.Undo, (_id: string, _token: string) => undefined),
  seed: bridged(WorldsGo.Seed, (_id: string, _folder: string) => '-4172144997902289642'),
  showInFolder: bridged(WorldsGo.ShowInFolder, (_id: string, _folder: string) => undefined),
  openFolder: bridged(WorldsGo.OpenFolder, (_id: string) => undefined),
};

export const serversApi = {
  /** force pings again instead of using the last minute's answers. */
  list: (id: string, force = false): Promise<ContentItem[]> => (hasBridge() ? revs.load('servers', () => serversList(id, force)) : preview(fixtures.servers)),
  /** The entry behind a row of the list (for the Edit dialog). */
  entry: (id: string, row: string): ServerEntry | undefined =>
    entries.get(`${id}/${row}`) ?? (hasBridge() ? undefined : (() => {
      const r = fixtures.servers.find((s) => s.id === row);
      return r ? { index: Number(row), name: r.name, address: r.sub.split(',')[0] ?? '', packs: 'prompt' as const } : undefined;
    })()),
  check: bridged(ServersGo.Check, (_id: string, _address: string): servers.Status => ({ online: true, version: '1.21.4', players: 6, max: 20, pingMs: 21 })),
  add: bridged(ServersGo.Add, (_id: string, _e: Omit<ServerEntry, 'index'>) => undefined),
  edit: bridged(ServersGo.Edit, (_id: string, _index: number, _address: string, _e: Omit<ServerEntry, 'index'>) => undefined),
  move: bridged(ServersGo.Move, (_id: string, _index: number, _address: string, _by: number) => undefined),
  remove: bridged(ServersGo.Remove, (_id: string, _index: number, _address: string) => 'preview'),
  undo: bridged(ServersGo.Undo, (_id: string, _token: string) => undefined),
};

export const screenshotsApi = {
  list: (id: string): Promise<Screenshot[]> =>
    hasBridge() ? revs.load('screenshots', () => ScreenshotsGo.List(id).then((l) => l.map(toShot))) : preview(home.instanceShots),
  copy: bridged(ScreenshotsGo.Copy, (_id: string, _name: string) => undefined),
  showInFolder: bridged(ScreenshotsGo.ShowInFolder, (_id: string, _name: string) => undefined),
  openFolder: bridged(ScreenshotsGo.OpenFolder, (_id: string) => undefined),
  useAsCover: bridged(ScreenshotsGo.UseAsCover, (_id: string, _name: string) => undefined),
  trash: bridged(ScreenshotsGo.Trash, (_id: string, _names: string[]) => 'preview'),
  undo: bridged(ScreenshotsGo.Undo, (_id: string, _token: string) => undefined),
};

export const homeApi = {
  news: bridged(
    () => HomeGo.News().then((n): NewsItem | null => (n ? { id: n.id, title: n.title, source: n.source, published: n.published, url: n.url, ...(n.image ? { image: n.image } : {}) } : null)).catch(() => null),
    (): NewsItem | null => home.news,
  ),
  week: () => revs.load('instances', bridged(HomeGo.Week, (): DayPlay[] => home.week)),
  servers: (force = false) => revs.load('servers', bridged(() => HomeGo.Servers(force).then((l) => l.map(toMerged)), () => home.servers)),
  recentShots: () => revs.load('screenshots', bridged(() => HomeGo.RecentShots().then((l) => l.map(toShot)), () => home.recentShots)),
  disk: bridged(HomeGo.Disk, (): DiskSpace => ({ free: 120e9, low: false })),
  /** What's wrong with where the data folder is: "bang" ("!" in the path), "temp", or "". */
  folderProblem: bridged(HomeGo.FolderProblem, () => ''),
};

export const logFilesApi = {
  files: (id: string) => bridged(() => LogsGo.Files(id) as Promise<LogFile[]>, () => logs.logFiles)(),
  text: bridged(LogsGo.Text, (_id: string, _name: string) => logs.latestLogText),
  runText: bridged(LogsGo.RunText, (_id: string, _run: string, _kind: 'log' | 'cmd' | 'crash') => logs.latestLogText),
  crash: bridged((id: string, run: string) => LogsGo.Crash(id, run) as Promise<CrashInfo>, (id: string, run: string) => logs.crash(id, run)),
  crashFile: bridged((id: string, name: string) => LogsGo.CrashFile(id, name) as Promise<CrashInfo>, (id: string, _name: string) => logs.crash(id, '')),
  uploadFile: bridged(LogsGo.UploadFile, (_id: string, _name: string) => 'https://mclo.gs/preview'),
  uploadRun: bridged(LogsGo.UploadRun, (_id: string, _run: string) => 'https://mclo.gs/preview'),
  debugInfo: bridged(LogsGo.DebugInfo, (_id: string, _run: string) => 'Quartz Client (preview)'),
  oldSize: bridged(LogsGo.OldSize, (_id: string) => 1_400_000),
  deleteOld: bridged(LogsGo.DeleteOld, (_id: string) => 1_400_000),
};

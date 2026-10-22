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
import type { Art, ContentItem, CrashInfo, DayPlay, DiskSpace, LogFile, MergedServer, NewsItem, Screenshot, ServerEntry, ServerStats } from './types';

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
  enabled: true, group: w.mode,
});

const capital = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

/** What both server lists show of a ping: the version to show and how it fits, players and ping. */
const serverStats = (s: Pick<GoServer, 'online' | 'version' | 'fit' | 'fitVersion' | 'players' | 'max' | 'pingMs' | 'sample'>): ServerStats => ({
  online: s.online, version: s.fitVersion || s.version || '', fit: (s.fit ?? '') as ServerStats['fit'],
  ...(s.online ? { players: { online: s.players, ...(s.max ? { max: s.max } : {}), ...(s.sample?.length ? { sample: s.sample } : {}) }, ...(s.pingMs ? { pingMs: s.pingMs } : {}) } : {}),
});

export const serverItem = (s: GoServer): ContentItem => ({
  id: String(s.index), name: s.name, sub: s.address, meta: '', art: serverArt(s.name, s.online, s.icon),
  enabled: true, online: s.online, server: serverStats(s),
});

/** What the server dialog edits, kept beside the rows by their id. */
const entries = new Map<string, ServerEntry>();

const toMerged = (m: GoMerged): MergedServer => ({
  id: m.id, name: m.name, address: m.address, packs: m.packs as ServerEntry['packs'], ...serverStats(m), art: serverArt(m.name, m.online, m.icon), instances: m.instances,
});

export const toShot = (s: GoShot): Screenshot => ({
  id: s.id, instanceId: s.instanceId, world: s.world, ...(s.server ? { server: s.server } : {}), title: s.title, takenAt: s.takenAt, file: s.file,
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
  /** Moves worlds aside; resolves with the token Undo takes to bring them all back. */
  remove: bridged(WorldsGo.Delete, (_id: string, _folders: string[]) => '0123456789abcdef'),
  undo: bridged(WorldsGo.Undo, (_id: string, _token: string) => undefined),
  seed: bridged(WorldsGo.Seed, (_id: string, _folder: string) => '-4172144997902289642'),
  showInFolder: bridged(WorldsGo.ShowInFolder, (_id: string, _folder: string) => undefined),
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
  /** Takes servers out (each by its place and address); resolves with the token Undo takes. */
  remove: bridged(ServersGo.Remove, (_id: string, _refs: servers.Ref[]) => 'preview'),
  undo: bridged(ServersGo.Undo, (_id: string, _token: string) => undefined),
};

export const screenshotsApi = {
  list: (id: string): Promise<Screenshot[]> =>
    hasBridge() ? revs.load('screenshots', () => ScreenshotsGo.List(id).then((l) => l.map(toShot))) : preview(home.instanceShots),
  copy: bridged(ScreenshotsGo.Copy, (_id: string, _name: string) => undefined),
  showInFolder: bridged(ScreenshotsGo.ShowInFolder, (_id: string, _name: string) => undefined),
  useAsPicture: bridged(ScreenshotsGo.UseAsPicture, (_id: string, _name: string) => undefined),
  useAsBanner: bridged(ScreenshotsGo.UseAsBanner, (_id: string, _name: string) => undefined),
  trash: bridged(ScreenshotsGo.Trash, (_id: string, _names: string[]) => 'preview'),
  undo: bridged(ScreenshotsGo.Undo, (_id: string, _token: string) => undefined),
  /** How many screenshots are still PNGs, and their size (Settings → Storage). */
  pngs: () => revs.load('screenshots', bridged(ScreenshotsGo.PNGs, () => ({ count: 312, bytes: 2_840_000_000 }))),
  /** Re-saves every PNG screenshot as JPEG, as a job; resolves with its id ("" when there are none). */
  compressAll: bridged(ScreenshotsGo.CompressAll, () => ''),
};

export const homeApi = {
  /** The latest Minecraft article first, then the launcher's own posts. */
  news: bridged(
    () => HomeGo.News().then((l) => l.map((n): NewsItem => ({ id: n.id, title: n.title, source: n.source, published: n.published, url: n.url, ...(n.image ? { image: n.image } : {}) }))).catch((): NewsItem[] => []),
    (): NewsItem[] => home.news,
  ),
  week: () => revs.load('instances', bridged(HomeGo.Week, (): DayPlay[] => home.week)),
  servers: (force = false) => revs.load('servers', bridged(() => HomeGo.Servers(force).then((l) => l.map(toMerged)), () => home.servers)),
  /** Takes Home's rows out of every instance that lists them; each part comes back with servers.undo. */
  removeServers: bridged(HomeGo.RemoveServers, (_keys: string[]): servers.Removed[] => [{ instance: 'preview', token: 'preview' }]),
  /** Changes Home's row in every instance that lists it; resolves with the row's ID after the edit. */
  editServer: bridged(HomeGo.EditServer, (key: string, _e: Omit<ServerEntry, 'index'>) => key),
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

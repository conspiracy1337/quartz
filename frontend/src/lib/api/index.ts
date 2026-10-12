// The one API the UI talks to. In M0 every domain except app and settings is served from
// mock fixtures; each later milestone replaces one domain with its Go service without
// touching the screens.
import * as browse from './mock/browse';
import { content } from './mock/content';
import * as home from './mock/home';
import * as inst from './mock/instances';
import * as logs from './mock/logs';
import * as misc from './mock/misc';
import * as versions from './mock/versions';
import type { ContentKind, Instance, InstanceSummary, ProjectHit, ProjectType } from './types';
import { bridged, hasBridge, preview } from '../bridge';
import { revs } from '../revs.svelte';
import * as InstancesGo from '../wailsjs/go/api/Instances';
import * as JavaGo from '../wailsjs/go/api/Java';
import * as LaunchGo from '../wailsjs/go/api/Launch';
import * as LogsGo from '../wailsjs/go/api/Logs';
import * as VersionsGo from '../wailsjs/go/api/Versions';
import { accountsApi, signInAlerts } from './accounts';
import { downloadsApi } from './downloads';
import { app, settingsApi } from './system';

export type { ProjectPage } from './mock/browse';
export type { AppInfo, CustomCSSState, Settings } from './system';

const serve = preview;

/**
 * A domain whose Go service arrives in a later milestone: the browser preview shows the
 * design's fixtures, the app shows nothing rather than fake data.
 */
const unbuilt = <A extends unknown[], T>(mock: (...a: A) => T, empty: T) => (...a: A): Promise<T> => (hasBridge() ? Promise.resolve(empty) : serve(mock(...a)));

const summary = ({ id, name, art, minecraft, loader, kind, hasServers, lastPlayed, playtimeMin, running }: Instance): InstanceSummary =>
  ({ id, name, art, minecraft, kind, hasServers, playtimeMin, running, ...(loader ? { loader } : {}), ...(lastPlayed !== undefined ? { lastPlayed } : {}) });

const byId = (id: string) => inst.instances.find((i) => i.id === id) ?? inst.instances[0]!;

export const api = {
  app,
  settings: settingsApi,
  instances: {
    list: () => revs.load('instances', bridged(InstancesGo.List, () => inst.instances.map(summary))),
    get: (id: string) => revs.load('instances', bridged(() => InstancesGo.Get(id), () => byId(id))),
    /** The sidebar's recent list: the instances played or created last. */
    recent: () => revs.load('instances', bridged(() => InstancesGo.List().then((l) => l.slice(0, 4)), () => inst.recentIds.map((id) => summary(byId(id))))),
    /** The instance Browse installs into by default: the most recently played one. */
    first: async (): Promise<Instance | undefined> => {
      const [top] = await api.instances.list();
      return top ? api.instances.get(top.id) : undefined;
    },
    create: bridged(InstancesGo.Create, inst.mockInstances.create),
    updateSettings: bridged(InstancesGo.UpdateSettings, inst.mockInstances.updateSettings),
    openFolder: bridged(InstancesGo.OpenFolder, (_id: string) => undefined),
    activity: unbuilt((_id: string) => inst.activity, []),
    pendingUpdates: unbuilt((_id: string) => inst.pendingUpdates, []),
  },
  content: {
    list: unbuilt((_instanceId: string, kind: ContentKind) => content[kind], []),
    setEnabled: (_instanceId: string, kind: ContentKind, itemId: string, enabled: boolean) => {
      const item = content[kind].find((c) => c.id === itemId);
      if (item) item.enabled = enabled;
      return serve(undefined);
    },
  },
  home: {
    /** Only sign-in alerts are real so far; the others arrive with their milestones. */
    alerts: () => (hasBridge() ? accountsApi.list().then(signInAlerts) : serve(home.alerts)),
    news: unbuilt((): typeof home.news | null => home.news, null),
    week: unbuilt(() => home.week, home.week.map((d) => ({ ...d, minutes: 0 }))),
    servers: unbuilt(() => home.servers, []),
    recentShots: unbuilt(() => home.recentShots, []),
  },
  screenshots: { list: unbuilt((_instanceId: string) => home.instanceShots, []) },
  launch: {
    /** accountId "" plays with the default account; "Launch as" passes another one. */
    play: bridged(LaunchGo.Play, (_id: string, _accountId: string) => undefined),
    stop: bridged(LaunchGo.Stop, (_id: string) => undefined),
    cancelLaunch: bridged(LaunchGo.CancelLaunch, (_id: string) => undefined),
    status: bridged(LaunchGo.Status, () => ({ running: [] as string[], launching: [] as string[] })),
    /** Re-verifies an instance's files; resolves with the job id ("" when all fine). */
    repair: bridged(LaunchGo.Repair, (_id: string) => ''),
  },
  logs: {
    live: bridged(LogsGo.Live, (_instanceId: string) => logs.liveLog),
    files: unbuilt((_instanceId: string) => logs.logFiles, []),
    text: unbuilt((_instanceId: string, _name: string) => logs.latestLogText, ''),
    runs: bridged(LogsGo.Runs, (_instanceId: string) => logs.runs),
  },
  versions: {
    minecraft: bridged(VersionsGo.Minecraft, () => versions.families),
    /** Bytes the first launch of a version still downloads. */
    estimate: bridged(VersionsGo.Estimate, (_id: string) => 180_000_000),
    loaders: unbuilt((_minecraft: string) => versions.loaders, versions.loaders.filter((l) => l.kind === 'none')),
  },
  accounts: accountsApi,
  browse: {
    catalog: unbuilt((type: ProjectType) => browse.catalog[type], { categories: [] as string[], hits: [] as ProjectHit[] }),
    project: (_id: string) => serve(browse.iris),
  },
  downloads: downloadsApi,
  sync: {
    groups: unbuilt(() => ({ ...misc.sync, total: misc.totalInstances }), { files: [], folders: [], mods: [], custom: [], total: 0 }),
    keybinds: unbuilt(() => misc.keybinds, []),
  },
  java: {
    runtimes: bridged(JavaGo.Runtimes, () => misc.runtimes),
    scan: bridged(JavaGo.Scan, () => misc.runtimes),
    addFromFolder: bridged(JavaGo.AddFromFolder, () => misc.runtimes),
    /** Starts a download job for the Java the newest release needs; resolves with its id. */
    download: bridged(JavaGo.Download, () => ''),
  },
  review: {
    modUpdates: unbuilt((_instanceId: string) => misc.modUpdates, []),
    modpackDiff: unbuilt((_instanceId: string) => misc.modpackDiff, { ...misc.modpackDiff, summary: '', keptEdits: [], rows: [] }),
  },
  imports: { launchers: unbuilt(() => misc.otherLaunchers, []) },
};

export type Api = typeof api;

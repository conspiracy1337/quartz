// The one API the UI talks to. In M0 every domain except app and settings is served from
// mock fixtures; each later milestone replaces one domain with its Go service without
// touching the screens.
import * as accounts from './mock/accounts';
import * as browse from './mock/browse';
import { content } from './mock/content';
import * as home from './mock/home';
import * as inst from './mock/instances';
import * as logs from './mock/logs';
import * as misc from './mock/misc';
import * as versions from './mock/versions';
import type { ContentKind, Instance, InstanceSummary, ProjectType } from './types';
import { bridged, preview } from '../bridge';
import { revs } from '../revs.svelte';
import * as AccountsGo from '../wailsjs/go/api/Accounts';
import * as InstancesGo from '../wailsjs/go/api/Instances';
import * as JavaGo from '../wailsjs/go/api/Java';
import * as LaunchGo from '../wailsjs/go/api/Launch';
import * as LogsGo from '../wailsjs/go/api/Logs';
import * as VersionsGo from '../wailsjs/go/api/Versions';
import { downloadsApi } from './downloads';
import { app, settingsApi } from './system';

export type { ProjectPage } from './mock/browse';
export type { AppInfo, CustomCSSState, Settings } from './system';

const serve = preview;

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
    create: bridged(InstancesGo.Create, inst.mockInstances.create),
    updateSettings: bridged(InstancesGo.UpdateSettings, inst.mockInstances.updateSettings),
    openFolder: bridged(InstancesGo.OpenFolder, (_id: string) => undefined),
    activity: (_id: string) => serve(inst.activity),
    pendingUpdates: (_id: string) => serve(inst.pendingUpdates),
  },
  content: {
    list: (_instanceId: string, kind: ContentKind) => serve(content[kind]),
    setEnabled: (_instanceId: string, kind: ContentKind, itemId: string, enabled: boolean) => {
      const item = content[kind].find((c) => c.id === itemId);
      if (item) item.enabled = enabled;
      return serve(undefined);
    },
  },
  home: {
    alerts: () => serve(home.alerts),
    news: () => serve(home.news),
    week: () => serve(home.week),
    servers: () => serve(home.servers),
    recentShots: () => serve(home.recentShots),
  },
  screenshots: { list: (_instanceId: string) => serve(home.instanceShots) },
  launch: {
    play: bridged(LaunchGo.Play, (_id: string) => undefined),
    stop: bridged(LaunchGo.Stop, (_id: string) => undefined),
    cancelLaunch: bridged(LaunchGo.CancelLaunch, (_id: string) => undefined),
    status: bridged(LaunchGo.Status, () => ({ running: [] as string[], launching: [] as string[] })),
    /** Re-verifies an instance's files; resolves with the job id ("" when all fine). */
    repair: bridged(LaunchGo.Repair, (_id: string) => ''),
  },
  logs: {
    live: bridged(LogsGo.Live, (_instanceId: string) => logs.liveLog),
    files: (_instanceId: string) => serve(logs.logFiles),
    text: (_instanceId: string, _name: string) => serve(logs.latestLogText),
    runs: bridged(LogsGo.Runs, (_instanceId: string) => logs.runs),
  },
  versions: {
    minecraft: bridged(VersionsGo.Minecraft, () => versions.families),
    /** Bytes the first launch of a version still downloads. */
    estimate: bridged(VersionsGo.Estimate, (_id: string) => 180_000_000),
    loaders: (_minecraft: string) => serve(versions.loaders),
  },
  accounts: {
    list: () => revs.load('accounts', bridged(AccountsGo.List, accounts.mockAccounts.list)),
    addOffline: bridged(AccountsGo.AddOffline, accounts.mockAccounts.addOffline),
    setDefault: bridged(AccountsGo.SetDefault, accounts.mockAccounts.setDefault),
    remove: bridged(AccountsGo.Remove, accounts.mockAccounts.remove),
    skins: () => serve(accounts.skins),
    capes: () => serve({ capes: accounts.capes, worn: accounts.wornCape }),
  },
  browse: {
    catalog: (type: ProjectType) => serve(browse.catalog[type]),
    project: (_id: string) => serve(browse.iris),
  },
  downloads: downloadsApi,
  sync: {
    groups: () => serve({ ...misc.sync, total: misc.totalInstances }),
    keybinds: () => serve(misc.keybinds),
  },
  java: {
    runtimes: bridged(JavaGo.Runtimes, () => misc.runtimes),
    scan: bridged(JavaGo.Scan, () => misc.runtimes),
    addFromFolder: bridged(JavaGo.AddFromFolder, () => misc.runtimes),
    /** Starts a download job for the Java the newest release needs; resolves with its id. */
    download: bridged(JavaGo.Download, () => ''),
  },
  review: {
    modUpdates: (_instanceId: string) => serve(misc.modUpdates),
    modpackDiff: (_instanceId: string) => serve(misc.modpackDiff),
  },
  imports: { launchers: () => serve(misc.otherLaunchers) },
};

export type Api = typeof api;

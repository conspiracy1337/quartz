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
    list: () => serve(inst.instances.map(summary)),
    get: (id: string) => serve(byId(id)),
    recent: () => serve(inst.recentIds.map((id) => summary(byId(id)))),
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
  logs: {
    live: (_instanceId: string) => serve(logs.liveLog),
    files: (_instanceId: string) => serve(logs.logFiles),
    text: (_instanceId: string, _name: string) => serve(logs.latestLogText),
    runs: (_instanceId: string) => serve(logs.runs),
  },
  versions: {
    minecraft: bridged(VersionsGo.Minecraft, () => versions.families),
    loaders: (_minecraft: string) => serve(versions.loaders),
  },
  accounts: {
    list: () => serve(accounts.accounts),
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
  java: { runtimes: () => serve(misc.runtimes) },
  review: {
    modUpdates: (_instanceId: string) => serve(misc.modUpdates),
    modpackDiff: (_instanceId: string) => serve(misc.modpackDiff),
  },
  imports: { launchers: () => serve(misc.otherLaunchers) },
};

export type Api = typeof api;

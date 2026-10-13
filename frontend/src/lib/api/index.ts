// The one API the UI talks to. In M0 every domain except app and settings is served from
// mock fixtures; each later milestone replaces one domain with its Go service without
// touching the screens.
import * as home from './mock/home';
import type { PlayInto } from './types';
import * as inst from './mock/instances';
import * as logs from './mock/logs';
import * as misc from './mock/misc';
import * as versions from './mock/versions';
import type { Alert, Instance, InstanceSummary } from './types';
import { activityFeed } from '../activity';
import { bridged, hasBridge, preview } from '../bridge';
import { revs } from '../revs.svelte';
import * as InstancesGo from '../wailsjs/go/api/Instances';
import * as JavaGo from '../wailsjs/go/api/Java';
import * as LaunchGo from '../wailsjs/go/api/Launch';
import * as LogsGo from '../wailsjs/go/api/Logs';
import * as SyncGo from '../wailsjs/go/api/Sync';
import * as VersionsGo from '../wailsjs/go/api/Versions';
import { accountsApi, signInAlerts } from './accounts';
import { browseApi, importsApi, manualApi, packsApi } from './browse';
import { contentApi } from './content';
import { bytes } from '../format';
import { homeApi, logFilesApi, screenshotsApi, serversApi, worldsApi } from './game';
import { downloadsApi } from './downloads';
import { hostApi } from './host';
import { app, settingsApi } from './system';

export type { AppInfo, CustomCSSState, Settings } from './system';

const serve = preview;

/**
 * A domain whose Go service arrives in a later milestone: the browser preview shows the
 * design's fixtures, the app shows nothing rather than fake data.
 */
const unbuilt = <A extends unknown[], T>(mock: (...a: A) => T, empty: T) => (...a: A): Promise<T> => (hasBridge() ? Promise.resolve(empty) : serve(mock(...a)));

const summary = ({ id, name, art, icon, minecraft, loader, kind, group, tags, hasServers, lastPlayed, playtimeMin, running }: Instance): InstanceSummary =>
  ({ id, name, art, minecraft, kind, tags, hasServers, playtimeMin, running, ...(icon ? { icon } : {}), ...(group ? { group } : {}), ...(loader ? { loader } : {}), ...(lastPlayed !== undefined ? { lastPlayed } : {}) });

const byId = (id: string) => inst.instances.find((i) => i.id === id) ?? inst.instances[0]!;

const runs = bridged(LogsGo.Runs, (_instanceId: string) => logs.runs);

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
    rename: bridged(InstancesGo.Rename, inst.mockInstances.rename),
    setArt: bridged(InstancesGo.SetArt, inst.mockInstances.setArt),
    /** Opens a native picture picker; resolves unchanged when cancelled. */
    pickIcon: bridged(InstancesGo.PickIcon, (id: string) => byId(id)),
    /** Uses a picture chooseIcon returned ("" goes back to the tile). */
    setIcon: bridged(InstancesGo.SetIcon, inst.mockInstances.setIcon),
    /** A picture for an instance not created yet; "" when cancelled. */
    chooseIcon: bridged(InstancesGo.ChooseIcon, () => ''),
    setNotes: bridged(InstancesGo.SetNotes, inst.mockInstances.setNotes),
    /** Joins or leaves a shared target (its own items merge in; leaving keeps a copy). */
    setSync: bridged(InstancesGo.SetSync, inst.mockInstances.setSync),
    setSharedKeys: bridged(InstancesGo.SetSharedKeys, inst.mockInstances.setSharedKeys),
    setTags: bridged(InstancesGo.SetTags, inst.mockInstances.setTags),
    groups: () => revs.load('instances', bridged(InstancesGo.Groups, () => inst.groups)),
    setGroup: bridged(InstancesGo.SetGroup, inst.mockInstances.setGroup),
    createGroup: bridged(InstancesGo.CreateGroup, inst.mockInstances.createGroup),
    /** Minecraft version and loader build; kind "" removes the loader, version "" picks its newest stable build. */
    setVersion: bridged(InstancesGo.SetVersion, inst.mockInstances.setVersion),
    setVersionOptions: bridged(InstancesGo.SetVersionOptions, inst.mockInstances.setVersionOptions),
    trash: bridged(InstancesGo.Trash, inst.mockInstances.trash),
    trashList: () => revs.load('instances', bridged(InstancesGo.TrashList, () => inst.trash)),
    restore: bridged(InstancesGo.Restore, inst.mockInstances.restore),
    purge: bridged(InstancesGo.Purge, inst.mockInstances.purge),
    /** Picks a folder in a native dialog and moves the game folder there as a job; resolves with its id ("" if cancelled). */
    move: bridged(InstancesGo.Move, (_id: string) => ''),
    moveDefault: bridged(InstancesGo.MoveDefault, (_id: string) => ''),
    /** How many enabled mods have a build for another Minecraft version (and loader), or null without mods. */
    compatibility: (id: string, target: string, loader = '') => contentApi.compatibility(id, target, loader),
    /** Play sessions and content changes, newest first. */
    activity: (id: string) => (hasBridge()
      ? revs.load('content', () => Promise.all([runs(id), contentApi.changes(id)]).then(([r, changes]) => activityFeed(r, changes)))
      : serve(inst.activity)),
    pendingUpdates: (id: string) => contentApi.pending(id),
  },
  content: contentApi,
  home: {
    /** Sign-in alerts, missing mod dependencies, modpack updates and low disk space. */
    alerts: () => (hasBridge()
      ? Promise.all([accountsApi.list().then(signInAlerts), missingAlerts(), packAlerts(), diskAlerts()]).then((lists) => lists.flat())
      : serve(home.alerts)),
    ...homeApi,
  },
  screenshots: screenshotsApi,
  worlds: worldsApi,
  servers: serversApi,
  launch: {
    /** accountId "" plays with the default account; "Launch as" passes another one. */
    play: bridged(LaunchGo.Play, (_id: string, _accountId: string) => undefined),
    /** Opens straight into a world (its folder) or a server (its address). */
    playInto: bridged((id: string, accountId: string, into: PlayInto) => LaunchGo.PlayInto(id, accountId, into.kind, into.target), (_id: string, _accountId: string, _into: PlayInto) => undefined),
    stop: bridged(LaunchGo.Stop, (_id: string) => undefined),
    cancelLaunch: bridged(LaunchGo.CancelLaunch, (_id: string) => undefined),
    status: bridged(LaunchGo.Status, () => ({ running: [] as string[], launching: [] as string[] })),
    /** Re-verifies an instance's files; resolves with the job id ("" when all fine). */
    repair: bridged(LaunchGo.Repair, (_id: string) => ''),
  },
  host: hostApi,
  logs: {
    live: bridged(LogsGo.Live, (_instanceId: string) => logs.liveLog),
    runs,
    ...logFilesApi,
  },
  versions: {
    minecraft: bridged(VersionsGo.Minecraft, () => versions.families),
    /** Bytes the first launch still downloads for a version and loader build ("" kind is vanilla, "" build the newest stable). */
    estimate: bridged(VersionsGo.Estimate, (_minecraft: string, _loader: string, _build: string) => 180_000_000),
    loaders: bridged(VersionsGo.Loaders, (_minecraft: string) => versions.loaders),
  },
  accounts: accountsApi,
  browse: browseApi,
  imports: importsApi,
  modpacks: packsApi,
  manual: manualApi,
  downloads: downloadsApi,
  sync: {
    groups: () => revs.load('sync', bridged(SyncGo.Groups, misc.mockSync.groups)),
    /** Turns a target on, or off: every joined instance keeps a copy and the shared one goes to the trash. */
    setEnabled: bridged(SyncGo.SetEnabled, misc.mockSync.setEnabled),
    addCustom: bridged(SyncGo.AddCustom, misc.mockSync.addCustom),
    removeCustom: bridged(SyncGo.RemoveCustom, misc.mockSync.removeCustom),
    openFolder: bridged(SyncGo.OpenFolder, () => undefined),
    keybinds: () => revs.load('sync', bridged(SyncGo.Keybinds, misc.mockSync.keybinds)),
    keyCatalog: bridged(SyncGo.KeyCatalog, misc.mockSync.keyCatalog),
    addKey: bridged(SyncGo.AddKey, misc.mockSync.addKey),
    /** Binds a shared keybind to a key the page captured (KeyboardEvent.code or "Mouse<button>"). */
    setKey: bridged(SyncGo.SetKey, misc.mockSync.setKey),
    removeKey: bridged(SyncGo.RemoveKey, misc.mockSync.removeKey),
  },
  java: {
    runtimes: bridged(JavaGo.Runtimes, () => misc.runtimes),
    scan: bridged(JavaGo.Scan, () => misc.runtimes),
    addFromFolder: bridged(JavaGo.AddFromFolder, () => misc.runtimes),
    /** Starts a download job for the Java the newest release needs; resolves with its id. */
    download: bridged(JavaGo.Download, () => ''),
  },
  /** Other launchers to import from (first-run setup, M11). */
  launchers: unbuilt(() => misc.otherLaunchers, []),
};

export type Api = typeof api;

/** "Prominence II is missing Sodium": one alert per instance whose mods lack a dependency. */
async function missingAlerts(): Promise<Alert[]> {
  const [missing, list] = await Promise.all([contentApi.missing(), InstancesGo.List()]);
  return missing.flatMap((m): Alert[] => {
    const name = list.find((i) => i.id === m.instanceId)?.name;
    if (!name) return [];
    const more = m.more ? ` and ${m.more} more` : '';
    return [{ tone: 'warn', subject: name, text: `is missing ${m.dep}${more}`, action: 'Fix', target: { kind: 'instance', id: m.instanceId, tab: 'mods' } }];
  });
}

/** "Low disk space 3.2 GB left", when the data folder's drive has under 5 GB free. */
async function diskAlerts(): Promise<Alert[]> {
  const d = await homeApi.disk();
  return d.low && d.free >= 0 ? [{ tone: 'warn', subject: 'Low disk space', text: `${bytes(d.free)} left`, action: 'Clean up', target: { kind: 'settings', tab: 'storage' } }] : [];
}

/** "Create: Above and Beyond 1.4.1 is out": one alert per pack with a newer version. */
async function packAlerts(): Promise<Alert[]> {
  const updates = await packsApi.updates();
  return updates.map((u): Alert => ({ tone: 'info', subject: u.name, text: `${u.version} is out`, action: 'Review', target: { kind: 'modpack-update', id: u.instanceId } }));
}

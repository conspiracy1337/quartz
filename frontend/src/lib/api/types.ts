// Domain types shared by the mock API (M0) and the real Go services that replace it
// milestone by milestone. Timestamps are epoch milliseconds; sizes are bytes.

import type { PreviewOnly } from '../bridge';

export type Loader = 'fabric' | 'quilt' | 'forge' | 'neoforge';
/** An instance's loader: `version` is what is shown ("47.3.0"), `id` the build Go stores ("1.20.1-47.3.0"). */
export interface LoaderRef { kind: Loader; version: string; id: string }

/** A coloured tile: a solid tint or gradient, optionally with a letter on it. */
export interface Art { background: string; initial?: string }

export type InstanceKind = 'modded' | 'modpack' | 'vanilla';

export interface InstanceSummary {
  id: string;
  name: string;
  art: Art;
  /** A picture's sha256 (served at /tex/icon/<icon>.png). */
  icon?: string;
  /** A banner's sha256 (served at /tex/banner/<banner>.png), drawn on the Library card's cover. */
  banner?: string;
  minecraft: string;
  loader?: LoaderRef;
  kind: InstanceKind;
  tags: string[];
  hasServers: boolean;
  lastPlayed?: number;
  /** When its page was last opened (unix ms). */
  lastOpened?: number;
  created: number;
  playtimeMin: number;
  running: boolean;
  /** A custom client jar it runs in place of vanilla's. */
  clientJar?: ClientJarRef;
}

/** An instance's custom client jar, as its pages show it. */
export interface ClientJarRef { name: string; size: number }
/** A client jar picked for New instance: checked and stored; `minecraft` is the version it says it was built from ("" when it doesn't say). */
export interface ClientJarPick extends ClientJarRef { sha1: string; minecraft: string }

export interface EnvVar { name: string; value: string }
/** Per-instance settings (mirrors Go's instance.Settings). */
export interface InstanceSettings {
  java: { override: boolean; javaPath: string; memoryMinMb: number; memoryMaxMb: number; jvmArgs: string };
  /** openInto skips the title screen: a world (its folder) or a server (its address); kind "" opens the title screen. */
  window: { width: number; height: number; fullscreen: boolean; hideOnGameStart?: boolean; openInto?: { kind: '' | 'world' | 'server'; target: string } };
  commands: { preLaunch: string; wrapper: string; postExit: string; env: EnvVar[]; systemGlfw: boolean; systemOpenal: boolean };
  performance: { dedicatedGpu: boolean; gameMode: boolean; mangoHud: boolean };
  /** The account this instance always plays as ("" or absent: the default one). */
  account?: string;
}
/** What the New instance screen sends; an empty loaderVersion means the newest stable build. */
export interface CreateInstance { name: string; art: Art; icon: string; banner: string; minecraft: string; loader: string; loaderVersion: string; sandbox: boolean; memoryMB: number; shares: string[]; clientJar: string }
/** How an instance shows: tile colour and letter, picture and banner ("" for none). */
export interface Look { art: Art; icon: string; banner: string }
/** A modpack install's look; packIcon uses the pack's own icon in place of `icon`. */
export interface PackLook extends Look { packIcon: boolean }
export interface Instance extends InstanceSummary {
  sandboxed: boolean;
  /** The sandbox's rules: standard | strict | custom. */
  sandboxPreset: string;
  counts: Record<'mods' | 'resourcepacks' | 'shaders' | 'datapacks' | 'worlds' | 'servers', number>;
  java: string;
  memoryMB: { min: number; max: number };
  folder: string;
  /** The game folder was moved outside the data folder. */
  moved: boolean;
  /** The modpack it was installed from; `updatable` when its platform can change its version. */
  pack?: { name: string; version: string; updatable: boolean };
  /** What "Automatic" Java picks for this version, e.g. "Java 21". */
  javaAuto: string;
  notes: string;
  keepLoaderUpdated: boolean;
  followLatest: boolean;
  sync: InstanceSync;
  discordStatus: boolean;
  settings: InstanceSettings;
  /** The dedicated GPU's name (Linux), "" when there is none. */
  gpu: string;
  /** The Linux game helpers that are installed. */
  tools: { gameMode: boolean; mangoHud: boolean };
  /** Only from setVersion: the job moving the mods to the new version. */
  switch?: SwitchResult;
}

/** An instance in the launcher's trash. */
export interface TrashEntry { key: string; id: string; name: string; art: Art; icon?: string; minecraft: string; loader?: string; trashedAt: number }

export type ActivityKind = 'session' | 'change';
export interface ActivityEvent {
  kind: ActivityKind;
  icon: 'play' | 'arrow-up' | 'plus' | 'warn' | 'camera' | 'trash';
  tone: 'muted' | 'accent' | 'danger';
  title: string;
  detail: string;
  at: number;
}

export interface PendingUpdate { name: string; art: Art; from: string; to: string }

// ---- content tabs ----
export type ContentKind = 'mods' | 'resourcepacks' | 'shaders' | 'datapacks' | 'worlds' | 'servers';
export type Source = 'modrinth' | 'curseforge' | 'norisk' | 'file';
export type ContentStatus =
  | { kind: 'update'; version: string }
  | { kind: 'locked' }
  | { kind: 'unknown' }
  | { kind: 'outdated'; label: string }
  | { kind: 'world'; world: string }
  /**
   * A mod for another loader or Minecraft, a missing or wrong-version dependency, a conflict,
   * a duplicate, a library nothing needs, or a file that can't be read. `problem` names which
   * (loader | missing | conflict | version | game | duplicate | library | unreadable); a missing
   * dependency's mod id is `depId`.
   */
  | { kind: 'problem'; label: string; tone: 'warn' | 'danger' | 'info'; problem?: string; depId?: string };

export interface ContentItem {
  id: string;
  name: string;
  sub: string;
  meta: string;
  art: Art;
  source?: Source;
  enabled: boolean;
  status?: ContentStatus;
  /** Filter group: the world for data packs, the game mode for worlds. */
  group?: string;
  online?: boolean;
  /** Update checks skip it (its .pw.toml is pinned). */
  locked?: boolean;
  /** Its page on Modrinth or CurseForge, when identified. */
  project?: string;
  /** Its reference for the launcher's own project page (identified items). */
  ref?: string;
  /** The file's name, for search. */
  file?: string;
  /** A mod a local server leaves out. */
  clientOnly?: boolean;
  /** A server row's ping answer. */
  server?: ServerStats;
}

/** What a content tab shows besides its rows. */
export interface ContentInfo { worlds: string[] }
/** The outcome of dropping files on a content tab. */
export interface AddResult { added: string[]; rejected: { name: string; reason: string }[] }
/** The shared content store's size, and how much of it nothing uses. */
export interface StoreUsage { blobs: number; bytes: number; unused: number; unusedBytes: number }
/** How many enabled mods have a build for another version; `unknown` aren't on a platform. */
export interface Compat { matching: number; total: number; unknown?: number }
/** What a version switch did with the mods. */
export interface SwitchResult { job: string; updated: number; disabled: string[]; unknown: string[] }
/** A content change in the Overview feed (Go's log of additions, updates and removals). */
export interface ContentChange { at: number; kind: 'added' | 'updated' | 'removed'; title: string; detail: string }

// ---- home ----
export interface Alert { tone: 'warn' | 'info'; subject: string; text: string; action: string; target: AlertTarget }
export type AlertTarget =
  | { kind: 'sign-in'; accountId: string }
  | { kind: 'instance'; id: string; tab: 'mods' | 'settings' }
  | { kind: 'modpack-update'; id: string }
  | { kind: 'settings'; tab: 'storage' };
/** The latest minecraft.net article; `image` is its picture on Go's image route. */
export interface NewsItem { id?: string; title: string; source: string; published: number; url: string; image?: string }
/** Minutes played on one day of this week (0 is Monday). */
export interface DayPlay { day: number; minutes: number; today: boolean }
/** Free space on the data folder's drive; `low` under 5 GB. */
export interface DiskSpace { free: number; low: boolean }

/**
 * A server's answer to a ping, as both server lists show it. `version` is the one to show:
 * the instance's own when the server takes it (`fit` ok), else the newest the server takes,
 * `minor` when that shares the instance's major version and `major` when it doesn't.
 */
export interface ServerStats {
  online: boolean;
  version: string;
  fit: '' | 'ok' | 'minor' | 'major';
  players?: { online: number; max?: number };
  pingMs?: number;
}

export interface MergedServer extends ServerStats {
  id: string;
  name: string;
  address: string;
  art: Art;
  /** Instances that list this server, last used first. */
  instances: string[];
}

export interface Screenshot {
  id: string;
  instanceId: string;
  world: string;
  title: string;
  takenAt: number;
  /** The grid's picture (a thumbnail inside the app). */
  art: Art;
  /** The full-size picture, for the viewer (the app only). */
  image?: string;
  file: string;
  width: number;
  height: number;
  bytes: number;
}

/** A server-list entry as the Add and Edit dialog edits it. */
export interface ServerEntry { index: number; name: string; address: string; packs: 'prompt' | 'enabled' | 'disabled' }

/** Where a launch opens straight away. */
export interface PlayInto { kind: 'world' | 'server'; target: string }

// ---- logs ----
export type LogLevel = 'INFO' | 'WARN' | 'ERROR' | 'DEBUG';
export interface LogLine { time: string; level: LogLevel; thread: string; message: string; redacted?: boolean }
export interface LogFile { name: string; bytes: number; crash: boolean }
/** One running copy of an instance, a source of the live console. `started` is unix ms. */
export interface LiveCopy { runId: string; accountId: string; started: number }
/** The crash dialog: the likely cause (when a rule matched), the fixes it offers and the trace. */
export interface CrashInfo {
  instanceId: string;
  runId?: string;
  file?: string;
  lengthMin: number;
  exitCode: number;
  cause?: CrashCause;
  trace: string;
}
export interface CrashCause {
  title: string;
  text: string;
  detail: string;
  mod?: { item: string; name: string; version: string; icon?: string; update?: string; enabled: boolean };
  fixes: ('update' | 'disable')[];
}
export type RunResult = 'ok' | 'crash' | 'killed' | 'failed';
export interface Run {
  id: string; startedAt: number; lengthMin: number; exitCode: number; result: RunResult;
  /** What the run used. */
  java: string; memory: { min: number; max: number }; account: string; mods: number;
  /** The sandbox it ran in ("bubblewrap"); absent when it ran unsandboxed. */
  sandbox?: string;
  /** Filled in after the game exits: the world played, the server joined, screenshots taken, the crash. */
  world?: string; server?: string; shots?: number; crash?: { title: string; detail?: string; report: boolean };
}

// ---- versions ----
export interface McVersion { id: string; java: number; released: number; snapshot?: boolean }
export interface VersionFamily { id: string; note: string; releases: McVersion[]; snapshots: McVersion[] }
/** One loader build: `name` is shown, `id` is stored; `released` is 0 when the upstream doesn't say. */
export interface LoaderVersion { id: string; name: string; released: number }
export interface LoaderOption { kind: Loader | 'none'; name: string; note: string; stable: LoaderVersion[]; beta: LoaderVersion[] }

// ---- accounts and skins ----
export interface Account {
  id: string;
  name: string;
  kind: 'microsoft' | 'offline';
  status: 'valid' | 'expired' | 'signin' | 'offline';
  detail: string;
  isDefault: boolean;
  /** The default skin it wears while skinHash is "": key names defaultURL(key). */
  defaultSkin: { key: string; slim: boolean };
  /** The library skin worn ("" for the default skin). */
  skinHash: string;
  slim: boolean;
  /** Last automatic sign-in refresh (unix ms), Microsoft accounts only. */
  refreshedAt: number;
}
/** A library skin. Its PNG is skinURL(hash). */
export interface Skin { hash: string; name: string; slim: boolean; added: number; source: string }
/** An owned cape. Its PNG is textureURL(texture); preview fixtures carry a CSS `art` instead. */
export interface Cape { id: string; name: string; texture: string; art?: PreviewOnly<string> }
export interface Capes { capes: Cape[]; worn: string }
/** Why a sign-in failed, with a page that fixes it when there is one. */
export interface SignInProblem { reason: string; message: string; fixUrl: string }
/** Progress of the interactive Microsoft sign-in (Go's accounts.SignInEvent). */
export interface SignInEvent {
  flow: number;
  mode: 'browser' | 'code';
  step?: string;
  code?: string;
  url?: string;
  qr?: boolean[][];
  expiresAt?: number;
  done: boolean;
  accountId?: string;
  error?: SignInProblem;
}

// ---- browse (mirrors Go's browse, content.InstallPlan and modpack) ----
export type ProjectType = 'Mods' | 'Modpacks' | 'Resource packs' | 'Shaders' | 'Data packs' | 'Worlds';
/** Go's project types, in Browse's tab order. */
export type ProjectKind = 'mod' | 'modpack' | 'resourcepack' | 'shader' | 'datapack' | 'world';
/** A platform: Modrinth, CurseForge, or the built-in NoRisk source of NoRiskClient's own mods. */
export type Src = 'modrinth' | 'curseforge' | 'norisk';
/** One heading of Browse's category filters and its names. */
export interface CategoryGroup { title: string; names: string[] }
export interface SourceStats { downloads: number; updated: number }
/** One merged search result; `id` stays the same when a twin from the other platform merges in. */
export interface ProjectHit {
  id: string;
  ref: string;
  name: string;
  author: string;
  summary: string;
  icon?: string;
  /** The preview's painted tile (real rows draw their icon). */
  art?: PreviewOnly<Art>;
  modrinth?: SourceStats;
  curseforge?: SourceStats;
  norisk?: SourceStats;
  /** The target instance already has it. */
  installed?: boolean;
}
export type BrowseSort = 'relevance' | 'downloads' | 'updated';
export interface BrowseQuery {
  type: ProjectKind; text: string; sort: BrowseSort; instance: string;
  version: boolean; loader: boolean; categories: string[];
  /** A Minecraft version picked by hand (modpacks); "" for any. */
  game: string;
  /** Only projects for one side; "" for any. */
  side: '' | 'client' | 'server';
  hideAi: boolean; hideAds: boolean; hideInstalled: boolean;
}
/** A page of results: rows to append and rows already shown that changed. */
export interface BrowseResults {
  searchId: string; rows: ProjectHit[]; changed: ProjectHit[]; more: boolean;
  /** Platforms that couldn't be searched for this page. */
  failed: Src[];
}
/** A run of description text: plain, code, or a link; `project` names the project a link points at ("modrinth/mod/sodium"). */
export interface Span { text: string; code?: boolean; href?: string; project?: string }
/** A picture in a badge row; `href` is the link around it. */
export interface DocPic { src: string; alt?: string; href?: string }
/** A block of project description text. Never raw HTML. */
export type DocBlock =
  | { type: 'p' | 'h2' | 'pre'; spans: Span[] }
  | { type: 'ul' | 'ol'; items: Span[][] }
  | { type: 'img'; src: string; alt?: string }
  | { type: 'badges'; pics: DocPic[] };
/** A gallery picture: a same-origin picture, or (in the preview) a painted stand-in. */
export interface GalleryItem { src?: string; art?: PreviewOnly<string>; title: string }
export interface ProjectLink { icon: string; label: string; url: string }
/** A project as one platform shows it. */
export interface ProjectSide {
  name: string; author: string; summary: string; icon?: string; art?: PreviewOnly<Art>;
  downloads: number; updated: number; url: string; versions: number;
  badges: string[]; body: DocBlock[]; gallery: GalleryItem[]; links: ProjectLink[];
  blocked?: boolean;
}
export interface InstalledVersion { item: string; source: Src; versionId: string; version: string; published: number }
export interface ProjectPage {
  ref: string; type: ProjectKind; name: string;
  modrinth?: ProjectSide; curseforge?: ProjectSide; norisk?: ProjectSide;
  /** The platform shown and installed from by default: Modrinth unless CurseForge has a newer release. */
  preferred: Src;
  /** Why: on one platform only, the same latest release on both, or a newer one; '' when unknown. */
  reason: 'only' | 'same' | 'newer' | '';
  installed?: InstalledVersion;
  /** The release types the update channel offers ("release", "beta"). */
  channels: string[];
}
export interface ProjectVersion {
  id: string; number: string; channel: 'Release' | 'Beta' | 'Alpha'; minecraft: string; published: number; bytes: number; downloads: number;
  /** Loaders it runs on: mod loaders, or a shader pack's (iris, optifine, canvas). */
  loaders: string[];
  /** What the row's button does for the target; "" for the installed version. */
  action: '' | 'Install' | 'Update' | 'Switch';
}
export interface VersionNotes { id: string; number: string; published: number; items: string[] }
export interface InstallDep {
  id: string; name: string; summary?: string; icon?: string; art?: PreviewOnly<Art>; required: boolean; installed: boolean; size: number; blocked?: boolean;
  /** Installed but turned off: the install turns it on. */
  off?: boolean;
  /** Required, but no build fits this instance: it is left out. */
  unavailable?: boolean;
}
export interface InstallPlan {
  planId: string; name: string; kind: ContentKind; version: string; size: number;
  replaces?: string; blocked?: boolean; deps: InstallDep[]; mods: number;
}
export interface PackStats { mods: number; modBytes: number; files: number; folders: string[]; blocked: number; foreign: string[] }
/** The Import screen's Detected card. */
export interface ImportPreview {
  token: string; name: string; version: string; minecraft: string; loader: string; source: string;
  icon?: string; art?: PreviewOnly<Art>; stats: PackStats; warnings: string[];
  /** Files the pack lets the player choose; they install turned off unless picked. */
  optional: { path: string; name: string }[];
  /** An instance already installed from this pack: it can move to this version (`target`) instead. */
  existing?: { id: string; name: string; version: string; target: string };
}
export interface PackUpdate { instanceId: string; name: string; version: string }
export interface ManualItem { id: string; name: string; fileName: string; size: number; state: 'waiting' | 'fetching' | 'found' | 'skipped'; twin: boolean; page?: string; art?: PreviewOnly<Art> }
/** The blocked-downloads dialog of one job. */
export interface ManualView { jobId: string; folder: string; watching?: boolean; items: ManualItem[]; done: boolean }

// ---- downloads (mirrors Go's tasks.JobView and tasks.Snapshot) ----
export type JobState = 'queued' | 'running' | 'paused' | 'done' | 'failed' | 'cancelled';
/** Drives the dot colour of a step row. */
export type StepState = 'queued' | 'run' | 'done' | 'warn' | 'failed';
export interface JobStep { name: string; unit: 'bytes' | 'count'; done: number; total: number; state: StepState }
/** A job's tile; one for an instance also has its picture and loader ("none" for vanilla). */
export interface JobArt extends Art { icon?: string; loader?: string }
export interface JobMeta { title: string; subtitle: string; art: JobArt; kind: string }
export interface Job {
  id: string;
  meta: JobMeta;
  state: JobState;
  doneBytes: number;
  totalBytes: number;
  /** Bytes per second. */
  speed: number;
  /** 0 while unknown. */
  etaSec: number;
  steps: JobStep[];
  error?: string;
  retryable: boolean;
  createdAt: number;
  /** 0 while active. */
  finishedAt: number;
}
export interface TaskSnapshot { active: Job[]; speed: number; etaSec: number; historyRev: number }

// ---- syncing ----
export interface SyncEntry { id: string; name: string; path: string; users: number; enabled: boolean; available: boolean }
export interface SyncGroups { files: SyncEntry[]; folders: SyncEntry[]; mods: SyncEntry[]; custom: SyncEntry[]; total: number }
/** A shared keybind: the game's id (options.txt key_<id>) and its key's label. */
export interface Keybind { id: string; name: string; key: string }
/** A keybind an instance has, for "Add key"; category is "Minecraft" or the mod's name. */
export interface KeyChoice { id: string; name: string; category: string; key: string; shared: boolean }
/** Which shared targets an instance joins, and whether it takes the shared keybinds. */
export interface InstanceSync { join: string[]; keys: boolean }

// ---- java ----
/** An instance that follows the latest release and whose mods all support it. */
export interface FollowOffer { instanceId: string; name: string; minecraft: string }
/** One instance with updates, from a check across every instance. */
export interface BulkCheck { instanceId: string; name: string; updates: number }

// ---- export ----
export type ExportFormat = 'mrpack' | 'curseforge' | 'zip' | 'list';
/** A mod list as Markdown, as JSON, or stored online behind a code. */
export type ExportListFormat = 'md' | 'json' | 'code';
export interface ExportInclude { worlds: boolean; screenshots: boolean; options: boolean; servers: boolean; config: boolean }
/** What an exported pack is called (remembered per instance). */
export interface ExportMeta { name: string; version: string; summary?: string; author?: string }
export interface ExportPlan { format: ExportFormat; list: ExportListFormat; include: ExportInclude; meta?: ExportMeta }
/** One line of an export's contents: a file, or a folder's files counted together ("config/", "48 files"). */
export interface ExportEntry { name: string; detail?: string; bytes: number; source?: Src }
/** What an export holds and leaves out; bytes is the bundled size (a list: its file's). */
export interface ExportPreview { linked: ExportEntry[]; bundled: ExportEntry[]; listed: ExportEntry[]; unlisted: ExportEntry[]; off: ExportEntry[]; bytes: number }
/** A shared mod list's code and when it stops working (unix ms). */
export interface ExportCode { code: string; expiresAt: number }
/** A pack export's job, or a mod list's saved file name; empty when the save dialog was cancelled. */
export interface ExportResult { job?: string; file?: string; code?: ExportCode }

export interface JavaRuntime {
  id: string; major: number; name: string; source: 'Managed' | 'System' | 'SDKMAN' | 'Found'; path: string; usedBy: number;
  /** Downloaded by Quartz (Remove), added from a folder (Forget), or every instance's Java while automatic picking is off. */
  removable: boolean; forgettable: boolean; default: boolean;
}

// ---- review screens ----
export interface ChangelogVersion { id: string; date: number; items: string[] }
/** One update in a review; `versions` (its changelog) loads when the dialog opens. */
export interface ModUpdate {
  id: string; name: string; note: string; from: string; to: string; art: Art; size: number;
  versions: ChangelogVersion[];
  /** CurseForge forbids third-party downloads of it. */
  blocked?: boolean;
  dependency?: boolean;
  /** Its version list on its platform's site. */
  versionsUrl?: string;
  /** Its reference for the launcher's own project page. */
  ref?: string;
}
/** An update check: what to update, what was skipped because it is locked. */
export interface UpdateReview { checkId: string; updates: ModUpdate[]; locked: { id: string; name: string; version: string }[]; failed: string[] }
export type DiffKind = 'Added' | 'Updated' | 'Removed';
export interface ModpackDiffRow { name: string; version: string; kind: DiffKind }
export interface ModpackDiff { from: string; to: string; summary: string; keptEdits: string[]; rows: ModpackDiffRow[]; game?: string }
/** One version of an instance's pack; `offered` is the one the last update check found. */
export interface PackVersion { id: string; number: string; channel: string; minecraft: string; published: number; current: boolean; offered: boolean }

// ---- importing ----
/** Another launcher on this computer; instances counts those not brought over yet. */
export interface OtherLauncher { id: string; name: string; art: Art; summary: string; instances: number }
/** The Java versions an import would download first, and their size in bytes. */
export interface JavaNeed { majors: number[]; bytes: number }

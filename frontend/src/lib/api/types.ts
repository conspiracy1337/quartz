// Domain types shared by the mock API (M0) and the real Go services that replace it
// milestone by milestone. Timestamps are epoch milliseconds; sizes are bytes.

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
  /** A picture's sha256 (served at /tex/icon/<icon>.png), or in the design's fixtures a glyph name. */
  icon?: string;
  minecraft: string;
  loader?: LoaderRef;
  kind: InstanceKind;
  group?: string;
  tags: string[];
  hasServers: boolean;
  lastPlayed?: number;
  created: number;
  playtimeMin: number;
  running: boolean;
}

export interface EnvVar { name: string; value: string }
/** Per-instance settings (mirrors Go's instance.Settings). */
export interface InstanceSettings {
  java: { override: boolean; javaPath: string; memoryMinMb: number; memoryMaxMb: number; jvmArgs: string };
  /** openInto skips the title screen: a world (its folder) or a server (its address); kind "" opens the title screen. */
  window: { width: number; height: number; fullscreen: boolean; onGameStart: string; openInto?: { kind: '' | 'world' | 'server'; target: string } };
  commands: { preLaunch: string; wrapper: string; postExit: string; env: EnvVar[]; systemGlfw: boolean; systemOpenal: boolean };
  performance: { dedicatedGpu: boolean; gameMode: boolean; mangoHud: boolean };
  /** The account this instance always plays as ("" or absent: the default one). */
  account?: string;
}
/** What the New instance screen sends; an empty loaderVersion means the newest stable build. */
export interface CreateInstance { name: string; art: Art; icon: string; minecraft: string; loader: string; loaderVersion: string; group: string; sandbox: boolean; shares: string[] }
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
  /** What "Automatic" Java picks for this version, e.g. "Java 21". */
  javaAuto: string;
  notes: string;
  keepLoaderUpdated: boolean;
  followLatest: boolean;
  syncTargets: number;
  sync: InstanceSync;
  discordStatus: boolean;
  settings: InstanceSettings;
  /** The dedicated GPU's name (Linux), "" when there is none. */
  gpu: string;
  /** Only from setVersion: the job moving the mods to the new version. */
  switch?: SwitchResult;
}

export interface Group { name: string; collapsed?: boolean }
/** An instance in the launcher's trash. */
export interface TrashEntry { key: string; id: string; name: string; art: Art; icon?: string; minecraft: string; loader?: string; group?: string; trashedAt: number }

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
export type Source = 'modrinth' | 'curseforge' | 'file';
export type ContentStatus =
  | { kind: 'update'; version: string }
  | { kind: 'locked' }
  | { kind: 'unknown' }
  | { kind: 'outdated'; label: string }
  | { kind: 'world'; world: string }
  | { kind: 'offline' }
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
  action?: 'Play' | 'Join';
  /** Filter group: the world for data packs, the game mode for worlds. */
  group?: string;
  online?: boolean;
  /** Update checks skip it (its .pw.toml is pinned). */
  locked?: boolean;
  /** Its page on Modrinth or CurseForge, when identified. */
  project?: string;
  /** Its reference for the launcher's own project page (identified items). */
  ref?: string;
  /** The file's name, its size in bytes and when it appeared (unix seconds), for search and sorting. */
  file?: string;
  size?: number;
  added?: number;
  /** A mod a local server leaves out. */
  clientOnly?: boolean;
}

/** What a content tab shows besides its rows. */
export interface ContentInfo { shaderMod: string; worlds: string[] }
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
  | { kind: 'instance'; id: string; tab: 'mods' | 'version' }
  | { kind: 'modpack-update'; id: string }
  | { kind: 'settings'; tab: 'storage' };
/** The latest minecraft.net article; `image` is its picture on Go's image route. */
export interface NewsItem { id?: string; title: string; source: string; published: number; url: string; image?: string }
/** Minutes played on one day of this week (0 is Monday). */
export interface DayPlay { day: number; minutes: number; today?: boolean }
/** Free space on the data folder's drive; `low` under 5 GB. */
export interface DiskSpace { free: number; low: boolean }

export interface MergedServer {
  id: string;
  name: string;
  address: string;
  version: string;
  online: boolean;
  players?: { online: number; max?: number };
  pingMs?: number;
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
  /** What the run used (recorded by Go; absent in older fixtures). */
  java?: string; memory?: { min: number; max: number }; account?: string; mods?: number;
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
export interface SkinColors { skin: string; hair: string; eye: string; mouth: string; top: string; topD: string; pants: string; shoes: string }
export interface Account {
  id: string;
  name: string;
  kind: 'microsoft' | 'offline';
  status: 'ok' | 'expired' | 'offline';
  detail: string;
  isDefault: boolean;
  /** The flat drawing, used while there is no skin texture. */
  skin: SkinColors;
  /** The library skin worn ("" for the default Steve or Alex). */
  skinHash: string;
  slim: boolean;
  /** Last automatic sign-in refresh (unix ms), Microsoft accounts only. */
  refreshedAt: number;
}
/** A library skin. Its PNG is skinURL(hash); preview fixtures carry flat colours instead. */
export interface Skin { hash: string; name: string; slim: boolean; added: number; source: string; colors?: SkinColors }
/** An owned cape. Its PNG is textureURL(texture); preview fixtures carry a CSS `art` instead. */
export interface Cape { id: string; name: string; texture: string; art?: string }
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
export type ProjectType = 'Mods' | 'Modpacks' | 'Resource packs' | 'Shaders' | 'Data packs';
/** Go's project types, in Browse's tab order. */
export type ProjectKind = 'mod' | 'modpack' | 'resourcepack' | 'shader' | 'datapack';
export type Src = 'modrinth' | 'curseforge';
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
  art?: Art;
  modrinth?: SourceStats;
  curseforge?: SourceStats;
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
export interface GalleryItem { src?: string; art?: string; title: string }
export interface ProjectLink { icon: string; label: string; url: string }
/** A project as one platform shows it. */
export interface ProjectSide {
  name: string; author: string; summary: string; icon?: string; art?: Art;
  downloads: number; updated: number; url: string; versions: number;
  badges: string[]; body: DocBlock[]; gallery: GalleryItem[]; links: ProjectLink[];
  blocked?: boolean;
}
export interface InstalledVersion { item: string; source: Src; versionId: string; version: string; published: number }
export interface ProjectPage {
  ref: string; type: ProjectKind; name: string;
  modrinth?: ProjectSide; curseforge?: ProjectSide;
  fresher: Src;
  installed?: InstalledVersion;
  /** The release types the update channel offers ("release", "beta"). */
  channels: string[];
}
export interface ProjectVersion {
  id: string; number: string; channel: 'Release' | 'Beta' | 'Alpha'; minecraft: string; published: number; bytes: number;
  /** What the row's button does for the target; "" for the installed version. */
  action: '' | 'Install' | 'Update' | 'Switch';
}
export interface VersionNotes { id: string; number: string; published: number; items: string[] }
export interface InstallDep {
  id: string; name: string; summary?: string; icon?: string; art?: Art; required: boolean; installed: boolean; size: number; blocked?: boolean;
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
  icon?: string; art?: Art; stats: PackStats; warnings: string[];
}
export interface PackUpdate { instanceId: string; name: string; version: string }
export interface ManualItem { id: string; name: string; fileName: string; size: number; state: 'waiting' | 'fetching' | 'found' | 'skipped'; twin: boolean; page?: string; art?: Art }
/** The blocked-downloads dialog of one job. */
export interface ManualView { jobId: string; folder: string; watching?: boolean; items: ManualItem[]; done: boolean }

// ---- downloads (mirrors Go's tasks.JobView and tasks.Snapshot) ----
export type JobState = 'queued' | 'running' | 'paused' | 'done' | 'failed' | 'cancelled';
/** Drives the dot colour of a step row. */
export type StepState = 'queued' | 'run' | 'done' | 'warn' | 'failed';
export interface JobStep { name: string; unit: 'bytes' | 'count'; done: number; total: number; state: StepState }
export interface JobMeta { title: string; subtitle: string; art: Art; kind: string }
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
export type ExportFormat = 'mrpack' | 'zip' | 'list';
export type ExportListFormat = 'md' | 'csv';
export interface ExportInclude { worlds: boolean; screenshots: boolean; options: boolean; servers: boolean; config: boolean }
export interface ExportPlan { format: ExportFormat; list: ExportListFormat; include: ExportInclude }
export interface ExportPreview { linked: number; bundled: number; bytes: number }
/** A pack export's job, or a mod list's saved file name; empty when the save dialog was cancelled. */
export interface ExportResult { job?: string; file?: string }

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

// ---- importing ----
/** Another launcher on this computer; instances counts those not brought over yet. */
export interface OtherLauncher { id: string; name: string; art: Art; summary: string; instances: number }
/** The Java versions an import would download first, and their size in bytes. */
export interface JavaNeed { majors: number[]; bytes: number }

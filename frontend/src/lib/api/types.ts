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
  playtimeMin: number;
  running: boolean;
}

export interface EnvVar { name: string; value: string }
/** Per-instance settings (mirrors Go's instance.Settings). */
export interface InstanceSettings {
  java: { override: boolean; javaPath: string; memoryMinMb: number; memoryMaxMb: number; jvmArgs: string };
  window: { width: number; height: number; fullscreen: boolean; onGameStart: string };
  commands: { preLaunch: string; wrapper: string; postExit: string; env: EnvVar[]; systemGlfw: boolean; systemOpenal: boolean };
  performance: { dedicatedGpu: boolean; gameMode: boolean; mangoHud: boolean };
}
/** What the New instance screen sends; an empty loaderVersion means the newest stable build. */
export interface CreateInstance { name: string; art: Art; icon: string; minecraft: string; loader: string; loaderVersion: string; group: string; sandbox: boolean; shares: string[] }
export interface Instance extends InstanceSummary {
  sandboxed: boolean;
  counts: Record<'mods' | 'resourcepacks' | 'shaders' | 'datapacks' | 'worlds' | 'servers', number>;
  java: string;
  memoryMB: { min: number; max: number };
  created: number;
  folder: string;
  /** The game folder was moved outside the data folder. */
  moved: boolean;
  /** What "Automatic" Java picks for this version, e.g. "Java 21". */
  javaAuto: string;
  notes: string;
  keepLoaderUpdated: boolean;
  followLatest: boolean;
  syncTargets: number;
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
  /** A missing dependency, a conflict, a duplicate, or a library nothing needs. */
  | { kind: 'problem'; label: string; tone: 'warn' | 'danger' | 'info' };

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
export type AlertTarget = { kind: 'sign-in'; accountId: string } | { kind: 'instance'; id: string; tab: 'mods' } | { kind: 'modpack-update'; id: string };
export interface NewsItem { title: string; source: string; published: number; url: string }
export interface DayPlay { day: number; minutes: number }

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
  art: Art;
  file: string;
  width: number;
  height: number;
  bytes: number;
}

// ---- logs ----
export type LogLevel = 'INFO' | 'WARN' | 'ERROR' | 'DEBUG';
export interface LogLine { time: string; level: LogLevel; thread: string; message: string; redacted?: boolean }
export interface LogFile { name: string; bytes: number; crash: boolean }
export type RunResult = 'ok' | 'crash' | 'killed' | 'failed';
export interface Run {
  id: string; startedAt: number; lengthMin: number; exitCode: number; result: RunResult;
  /** What the run used (recorded by Go; absent in older fixtures). */
  java?: string; memory?: { min: number; max: number }; account?: string; mods?: number;
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

// ---- browse ----
export type ProjectType = 'Mods' | 'Modpacks' | 'Resource packs' | 'Shaders' | 'Data packs';
export interface SourceStats { downloads: number; updated: number }
export interface ProjectHit {
  id: string;
  name: string;
  author: string;
  description: string;
  art: Art;
  modrinth?: SourceStats;
  curseforge?: SourceStats;
}
/** A block of project description text. Inline `code` is marked with backticks. */
export type DocBlock = { type: 'p' | 'h2'; text: string } | { type: 'ul'; items: string[] };
export interface ProjectVersion { id: string; channel: 'Release' | 'Beta' | 'Alpha'; minecraft: string; published: number; bytes: number; notes: string[] }

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
export interface SyncGroups { files: SyncEntry[]; folders: SyncEntry[]; mods: SyncEntry[]; custom: SyncEntry[] }
export interface Keybind { name: string; key: string }

// ---- java ----
export interface JavaRuntime { major: number; name: string; source: 'Managed' | 'System' | 'SDKMAN' | 'Found'; path: string; usedBy: number }

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
}
/** An update check: what to update, what was skipped because it is locked. */
export interface UpdateReview { checkId: string; updates: ModUpdate[]; locked: { id: string; name: string; version: string }[]; failed: string[] }
export type DiffKind = 'Added' | 'Updated' | 'Removed';
export interface ModpackDiffRow { name: string; version: string; kind: DiffKind }

// ---- importing ----
export interface OtherLauncher { name: string; art: Art; summary: string; instances: number }

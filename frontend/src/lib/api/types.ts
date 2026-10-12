// Domain types shared by the mock API (M0) and the real Go services that replace it
// milestone by milestone. Timestamps are epoch milliseconds; sizes are bytes.

export type Loader = 'fabric' | 'quilt' | 'forge' | 'neoforge';
export interface LoaderRef { kind: Loader; version: string }

/** A coloured tile: a solid tint or gradient, optionally with a letter on it. */
export interface Art { background: string; initial?: string }

export type InstanceKind = 'modded' | 'modpack' | 'vanilla';

export interface InstanceSummary {
  id: string;
  name: string;
  art: Art;
  minecraft: string;
  loader?: LoaderRef;
  kind: InstanceKind;
  hasServers: boolean;
  lastPlayed?: number;
  playtimeMin: number;
  running: boolean;
}

/** Per-instance settings (mirrors Go's instance.Settings). */
export interface InstanceSettings {
  java: { override: boolean; javaPath: string; memoryMinMb: number; memoryMaxMb: number; jvmArgs: string };
  window: { width: number; height: number; fullscreen: boolean; onGameStart: string };
}
/** What the New instance screen sends. */
export interface CreateInstance { name: string; art: Art; minecraft: string; loader: string; group: string; sandbox: boolean; shares: string[] }
export interface Instance extends InstanceSummary {
  icon?: 'bolt';
  sandboxed: boolean;
  counts: Record<'mods' | 'resourcepacks' | 'shaders' | 'datapacks' | 'worlds' | 'servers', number>;
  java: string;
  memoryMB: { min: number; max: number };
  created: number;
  folder: string;
  syncTargets: number;
  discordStatus: boolean;
  settings: InstanceSettings;
}

export type ActivityKind = 'session' | 'change';
export interface ActivityEvent {
  kind: ActivityKind;
  icon: 'play' | 'arrow-up' | 'plus' | 'warn' | 'camera';
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
  | { kind: 'offline' };

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
}

// ---- home ----
export interface Alert { tone: 'warn' | 'info'; subject: string; text: string; action: string; target: AlertTarget }
export type AlertTarget = { kind: 'sign-in' } | { kind: 'instance'; id: string; tab: 'mods' } | { kind: 'modpack-update'; id: string };
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
export interface Run { id: string; startedAt: number; lengthMin: number; exitCode: number; result: RunResult }

// ---- versions ----
export interface McVersion { id: string; java: number; released: number; snapshot?: boolean }
export interface VersionFamily { id: string; note: string; releases: McVersion[]; snapshots: McVersion[] }
export interface LoaderVersion { id: string; released: number }
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
  skin: SkinColors;
}
export interface Skin { id: string; name: string; colors: SkinColors; slim: boolean; worn: boolean }
export interface Cape { id: string; name: string; art: string }

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
export interface ModUpdate { name: string; note: string; from: string; to: string; art: Art; versions: { id: string; date: number; items: string[] }[] }
export type DiffKind = 'Added' | 'Updated' | 'Removed';
export interface ModpackDiffRow { name: string; version: string; kind: DiffKind }

// ---- importing ----
export interface OtherLauncher { name: string; art: Art; summary: string; instances: number }

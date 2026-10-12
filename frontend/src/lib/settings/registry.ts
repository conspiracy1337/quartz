// Every global setting is defined once here. The settings tabs render their groups from
// this list and the settings search filters it, so a setting can never appear in two
// places with two different labels or behaviours.
import type { Settings } from '../api';
import type { Route, SettingsTab } from '../router.svelte';

type Opt = { value: string; label: string };
export type Control =
  | { kind: 'toggle'; get: (s: Settings) => boolean; set: (s: Settings, v: boolean) => void }
  | { kind: 'seg'; options: readonly Opt[]; get: (s: Settings) => string; set: (s: Settings, v: string) => void }
  | { kind: 'select'; options: readonly Opt[]; get: (s: Settings) => string; set: (s: Settings, v: string) => void }
  | { kind: 'range'; min: number; max: number; get: (s: Settings) => number; set: (s: Settings, v: number) => void }
  | { kind: 'memory' }
  | { kind: 'textarea'; rows: number; get: (s: Settings) => string; set: (s: Settings, v: string) => void }
  | { kind: 'link'; label: string; route: Route };

export interface SettingDef {
  id: string;
  tab: SettingsTab;
  /** Section heading on its page; also the small label in search results. */
  group: string;
  title: string;
  desc?: string;
  control: Control;
}

// Equivalent options use the same labels everywhere (global and instance settings).
export const ON_GAME_START = [{ value: 'keep-open', label: 'Keep open' }, { value: 'hide', label: 'Hide' }, { value: 'close', label: 'Close' }] as const;
export const CHANNELS = [{ value: 'release', label: 'Release' }, { value: 'beta', label: 'Beta' }, { value: 'alpha', label: 'Alpha' }] as const;

export const registry: SettingDef[] = [
  { id: 'language', tab: 'general', group: 'Launcher', title: 'Language', control: { kind: 'select', options: [{ value: 'system', label: 'English (system)' }, { value: 'en', label: 'English' }, { value: 'de', label: 'Deutsch' }], get: (s) => s.general.language, set: (s, v) => (s.general.language = v) } },
  { id: 'onGameStart', tab: 'general', group: 'Launcher', title: 'When a game starts', desc: 'Instances can override this.', control: { kind: 'seg', options: ON_GAME_START, get: (s) => s.general.onGameStart, set: (s, v) => (s.general.onGameStart = v) } },
  { id: 'startPage', tab: 'general', group: 'Launcher', title: 'Open on startup', control: { kind: 'seg', options: [{ value: 'home', label: 'Home' }, { value: 'library', label: 'Library' }, { value: 'last-instance', label: 'Last instance' }], get: (s) => s.general.startPage, set: (s, v) => (s.general.startPage = v) } },
  { id: 'startWithComputer', tab: 'general', group: 'Launcher', title: 'Start with the computer', desc: 'Starts minimized so updates are ready.', control: { kind: 'toggle', get: (s) => s.general.startWithComputer, set: (s, v) => (s.general.startWithComputer = v) } },
  { id: 'setup', tab: 'general', group: 'Launcher', title: 'First-run setup', desc: 'Walk through sign-in and importing from other launchers again.', control: { kind: 'link', label: 'Run again', route: { name: 'setup', step: 1 } } },

  { id: 'parallel', tab: 'downloads', group: 'Downloads', title: 'Parallel downloads', control: { kind: 'range', min: 1, max: 16, get: (s) => s.downloads.parallel, set: (s, v) => (s.downloads.parallel = v) } },
  { id: 'watchDownloads', tab: 'downloads', group: 'Downloads', title: 'Watch the Downloads folder', desc: 'Picks up CurseForge files that must be downloaded by hand.', control: { kind: 'toggle', get: (s) => s.downloads.watchDownloads, set: (s, v) => (s.downloads.watchDownloads = v) } },
  { id: 'proxy', tab: 'downloads', group: 'Downloads', title: 'Proxy', control: { kind: 'select', options: [{ value: 'system', label: 'Use system settings' }, { value: 'none', label: 'No proxy' }, { value: 'custom', label: 'Custom proxy' }], get: (s) => s.downloads.proxy, set: (s, v) => (s.downloads.proxy = v) } },
  { id: 'modChannel', tab: 'downloads', group: 'Mod updates', title: 'Mod update channel', desc: 'The least stable versions to offer when checking for mod updates.', control: { kind: 'seg', options: CHANNELS, get: (s) => s.updates.modChannel, set: (s, v) => (s.updates.modChannel = v) } },

  { id: 'discord', tab: 'integrations', group: 'Integrations', title: 'Discord status', desc: 'Shows the instance and Minecraft version you are playing.', control: { kind: 'toggle', get: (s) => s.integrations.discordStatus, set: (s, v) => (s.integrations.discordStatus = v) } },
  { id: 'links', tab: 'integrations', group: 'Integrations', title: 'Open Modrinth and CurseForge links', desc: 'Handles modrinth:// and curseforge:// install buttons on the web.', control: { kind: 'toggle', get: (s) => s.integrations.handleLinks, set: (s, v) => (s.integrations.handleLinks = v) } },

  { id: 'density', tab: 'appearance', group: 'Layout', title: 'Density', control: { kind: 'seg', options: [{ value: 'comfortable', label: 'Comfortable' }, { value: 'compact', label: 'Compact' }], get: (s) => s.appearance.density, set: (s, v) => (s.appearance.density = v) } },
  { id: 'textSize', tab: 'appearance', group: 'Layout', title: 'Text size', control: { kind: 'seg', options: [{ value: 'small', label: 'Small' }, { value: 'default', label: 'Default' }, { value: 'large', label: 'Large' }], get: (s) => s.appearance.textSize, set: (s, v) => (s.appearance.textSize = v) } },
  { id: 'reduceMotion', tab: 'appearance', group: 'Layout', title: 'Reduce motion', desc: 'Follows your system setting by default.', control: { kind: 'select', options: [{ value: 'system', label: 'System' }, { value: 'on', label: 'On' }, { value: 'off', label: 'Off' }], get: (s) => s.appearance.reduceMotion, set: (s, v) => (s.appearance.reduceMotion = v) } },

  { id: 'autoJava', tab: 'java', group: 'Java', title: 'Pick Java automatically', desc: 'Uses the version each Minecraft release needs.', control: { kind: 'toggle', get: (s) => s.java.autoPick, set: (s, v) => (s.java.autoPick = v) } },
  { id: 'javaVendor', tab: 'java', group: 'Java', title: 'Download missing versions', desc: 'From', control: { kind: 'select', options: [{ value: 'temurin', label: 'Eclipse Temurin' }, { value: 'zulu', label: 'Azul Zulu' }, { value: 'mojang', label: 'Mojang' }], get: (s) => s.java.downloadVendor, set: (s, v) => (s.java.downloadVendor = v) } },
  { id: 'memory', tab: 'java', group: 'Java', title: 'Default memory', desc: 'Instances can override.', control: { kind: 'memory' } },
  { id: 'warnLowMemory', tab: 'java', group: 'Java', title: 'Warn when free memory is low', desc: 'Checks before each launch.', control: { kind: 'toggle', get: (s) => s.java.warnLowMemory, set: (s, v) => (s.java.warnLowMemory = v) } },
  { id: 'suggestMemory', tab: 'java', group: 'Instances', title: 'Suggest memory from mod count', desc: 'Shown in each instance’s settings.', control: { kind: 'toggle', get: (s) => s.java.suggestMemory, set: (s, v) => (s.java.suggestMemory = v) } },
  { id: 'jvmArgs', tab: 'java', group: 'Java', title: 'Default JVM arguments', control: { kind: 'textarea', rows: 2, get: (s) => s.java.jvmArgs, set: (s, v) => (s.java.jvmArgs = v) } },

  { id: 'autoUpdate', tab: 'updates', group: 'Updates', title: 'Automatic launcher updates', desc: 'Downloads signed updates in the background and installs them on restart.', control: { kind: 'toggle', get: (s) => s.updates.autoCheck, set: (s, v) => (s.updates.autoCheck = v) } },
];

/** The settings of one tab, grouped by section heading in registry order. */
export function groupsFor(tab: SettingsTab): { group: string; defs: SettingDef[] }[] {
  const out: { group: string; defs: SettingDef[] }[] = [];
  for (const d of registry.filter((r) => r.tab === tab)) {
    const g = out.find((x) => x.group === d.group) ?? out[out.push({ group: d.group, defs: [] }) - 1]!;
    g.defs.push(d);
  }
  return out;
}

/** Case-insensitive search over titles and descriptions. */
export function search(q: string): SettingDef[] {
  const n = q.trim().toLowerCase();
  if (!n) return [];
  return registry.filter((d) => d.title.toLowerCase().includes(n) || (d.desc ?? '').toLowerCase().includes(n) || d.group.toLowerCase().includes(n));
}

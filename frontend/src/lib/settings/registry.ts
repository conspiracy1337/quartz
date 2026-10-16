// Every global setting is defined once here. The settings tabs render their groups from
// this list and the settings search filters it, so a setting can never appear in two
// places with two different labels or behaviours.
import { settingsApi, type AppInfo, type Settings } from '../api/system';
import { gb, gbField } from '../format';
import type { Route, SettingsTab } from '../router.svelte';

type Opt = { value: string; label: string };
/** One text input inside an `inputs` control; `grow` is its share of the control column. */
export type Field = {
  label: string;
  get: (s: Settings) => string | number;
  set: (s: Settings, v: string) => void;
  numeric?: boolean;
  suffix?: string;
  placeholder?: string;
  grow?: number;
};
export type Control =
  | { kind: 'toggle'; get: (s: Settings) => boolean; set: (s: Settings, v: boolean) => void; disabled?: (info: AppInfo | undefined) => boolean }
  | { kind: 'seg'; options: readonly Opt[]; get: (s: Settings) => string; set: (s: Settings, v: string) => void }
  | { kind: 'select'; options: readonly Opt[]; get: (s: Settings) => string; set: (s: Settings, v: string) => void }
  | { kind: 'range'; min: number; max: number; get: (s: Settings) => number; set: (s: Settings, v: number) => void }
  | { kind: 'inputs'; fields: readonly Field[] }
  /** A write-only value kept outside the settings file (OS keychain). */
  | { kind: 'secret'; has: () => Promise<boolean>; save: (v: string) => Promise<void> }
  | { kind: 'textarea'; rows: number; get: (s: Settings) => string; set: (s: Settings, v: string) => void }
  | { kind: 'link'; label: string; route: Route };

export interface SettingDef {
  id: string;
  tab: SettingsTab;
  /** Section heading on its page; also the small label in search results. */
  group: string;
  title: string;
  /** Static text, or text built from facts about this PC (installed memory). */
  desc?: string | ((info: AppInfo | undefined) => string);
  control: Control;
  /** Shown only while this holds (e.g. custom proxy rows, or a setting only some systems have). */
  when?: (s: Settings, info: AppInfo | undefined) => boolean;
}

// Equivalent settings use the same wording everywhere (global and instance settings).
export const HIDE_ON_GAME_START = { title: 'Hide the launcher while playing', desc: 'It comes back when the game exits.' } as const;
const num = (v: string) => Number(v) || 0;
const customProxy = (s: Settings) => s.downloads.proxy === 'custom';

export const CHANNELS = [{ value: 'release', label: 'Release' }, { value: 'beta', label: 'Beta' }, { value: 'alpha', label: 'Alpha' }] as const;

export const registry: SettingDef[] = [
  { id: 'language', tab: 'general', group: 'Launcher', title: 'Language', control: { kind: 'select', options: [{ value: 'system', label: 'English' }], get: () => 'system', set: (s) => (s.general.language = 'system') } },
  { id: 'hideOnGameStart', tab: 'general', group: 'Launcher', title: HIDE_ON_GAME_START.title, desc: `${HIDE_ON_GAME_START.desc} Instances can override this.`, control: { kind: 'toggle', get: (s) => s.general.hideOnGameStart, set: (s, v) => (s.general.hideOnGameStart = v) } },
  { id: 'startPage', tab: 'general', group: 'Launcher', title: 'Open on startup', control: { kind: 'seg', options: [{ value: 'home', label: 'Home' }, { value: 'library', label: 'Library' }, { value: 'last-instance', label: 'Last instance' }], get: (s) => s.general.startPage, set: (s, v) => (s.general.startPage = v) } },
  { id: 'startWithComputer', tab: 'general', group: 'Launcher', title: 'Start with the computer', desc: 'Starts minimized so updates are ready.', control: { kind: 'toggle', get: (s) => s.general.startWithComputer, set: (s, v) => (s.general.startWithComputer = v) } },
  { id: 'setup', tab: 'general', group: 'Launcher', title: 'First-run setup', desc: 'Walk through sign-in and importing from other launchers again.', control: { kind: 'link', label: 'Run again', route: { name: 'setup', step: 1 } } },
  // Linux only for now: elsewhere the system offers no per-program volume to set.
  { id: 'muteUnfocused', tab: 'general', group: 'While playing', title: 'Mute the game when it isn’t focused', desc: (info) => (info && !info.muteUnfocused ? 'Needs an X or XWayland display and pactl.' : 'Silences a game while you use another window.'), when: (_s, info) => info?.os === 'linux', control: { kind: 'toggle', get: (s) => s.general.muteUnfocused, set: (s, v) => (s.general.muteUnfocused = v), disabled: (info) => !!info && !info.muteUnfocused } },

  { id: 'parallel', tab: 'downloads', group: 'Downloads', title: 'Parallel downloads', control: { kind: 'range', min: 1, max: 16, get: (s) => s.downloads.parallel, set: (s, v) => (s.downloads.parallel = v) } },
  { id: 'watchDownloads', tab: 'downloads', group: 'Downloads', title: 'Watch the Downloads folder', desc: 'Picks up CurseForge files that must be downloaded by hand.', control: { kind: 'toggle', get: (s) => s.downloads.watchDownloads, set: (s, v) => (s.downloads.watchDownloads = v) } },
  { id: 'proxy', tab: 'downloads', group: 'Downloads', title: 'Proxy', control: { kind: 'select', options: [{ value: 'system', label: 'Use system settings' }, { value: 'none', label: 'No proxy' }, { value: 'custom', label: 'Custom proxy' }], get: (s) => s.downloads.proxy, set: (s, v) => (s.downloads.proxy = v) } },
  { id: 'proxyType', tab: 'downloads', group: 'Downloads', title: 'Proxy type', when: customProxy, control: { kind: 'select', options: [{ value: 'http', label: 'HTTP' }, { value: 'https', label: 'HTTPS' }, { value: 'socks5', label: 'SOCKS5' }], get: (s) => s.downloads.proxyCustom.type, set: (s, v) => (s.downloads.proxyCustom.type = v) } },
  { id: 'proxyAddress', tab: 'downloads', group: 'Downloads', title: 'Proxy address', when: customProxy, control: { kind: 'inputs', fields: [
    { label: 'Host', placeholder: 'proxy.example.com', grow: 2, get: (s) => s.downloads.proxyCustom.host, set: (s, v) => (s.downloads.proxyCustom.host = v.trim()) },
    { label: 'Port', numeric: true, suffix: 'port', get: (s) => s.downloads.proxyCustom.port, set: (s, v) => (s.downloads.proxyCustom.port = num(v)) },
  ] } },
  { id: 'proxyUser', tab: 'downloads', group: 'Downloads', title: 'Proxy user name', desc: 'Leave empty if the proxy needs no sign-in.', when: customProxy, control: { kind: 'inputs', fields: [
    { label: 'User name', placeholder: 'Optional', get: (s) => s.downloads.proxyCustom.user, set: (s, v) => (s.downloads.proxyCustom.user = v) },
  ] } },
  { id: 'proxyPassword', tab: 'downloads', group: 'Downloads', title: 'Proxy password', desc: 'Kept in your system keychain, never in a file.', when: customProxy, control: { kind: 'secret', has: () => settingsApi.hasProxyPassword(), save: (v) => settingsApi.setProxyPassword(v) } },
  { id: 'modChannel', tab: 'downloads', group: 'Mod updates', title: 'Mod update channel', desc: 'The least stable versions to offer when checking for mod updates.', control: { kind: 'seg', options: CHANNELS, get: (s) => s.updates.modChannel, set: (s, v) => (s.updates.modChannel = v) } },

  { id: 'discord', tab: 'integrations', group: 'Integrations', title: 'Discord status', desc: (info) => (info && !info.discord ? 'Not set up in this build.' : 'Shows the instance and Minecraft version you are playing.'), control: { kind: 'toggle', get: (s) => s.integrations.discordStatus, set: (s, v) => (s.integrations.discordStatus = v), disabled: (info) => !!info && !info.discord } },
  { id: 'links', tab: 'integrations', group: 'Integrations', title: 'Open Modrinth and CurseForge links', desc: 'Handles modrinth:// and curseforge:// install buttons on the web.', control: { kind: 'toggle', get: (s) => s.integrations.handleLinks, set: (s, v) => (s.integrations.handleLinks = v) } },

  { id: 'density', tab: 'appearance', group: 'Layout', title: 'Density', control: { kind: 'seg', options: [{ value: 'comfortable', label: 'Comfortable' }, { value: 'compact', label: 'Compact' }], get: (s) => s.appearance.density, set: (s, v) => (s.appearance.density = v) } },
  { id: 'textSize', tab: 'appearance', group: 'Layout', title: 'Text size', control: { kind: 'seg', options: [{ value: 'small', label: 'Small' }, { value: 'default', label: 'Default' }, { value: 'large', label: 'Large' }], get: (s) => s.appearance.textSize, set: (s, v) => (s.appearance.textSize = v) } },
  { id: 'reduceMotion', tab: 'appearance', group: 'Layout', title: 'Reduce motion', desc: 'Follows your system setting by default.', control: { kind: 'select', options: [{ value: 'system', label: 'System' }, { value: 'on', label: 'On' }, { value: 'off', label: 'Off' }], get: (s) => s.appearance.reduceMotion, set: (s, v) => (s.appearance.reduceMotion = v) } },

  { id: 'autoJava', tab: 'java', group: 'Java', title: 'Pick Java automatically', desc: 'Uses the version each Minecraft release needs.', control: { kind: 'toggle', get: (s) => s.java.autoPick, set: (s, v) => (s.java.autoPick = v) } },
  { id: 'javaVendor', tab: 'java', group: 'Java', title: 'Download missing versions', desc: 'From', control: { kind: 'select', options: [{ value: 'temurin', label: 'Eclipse Temurin' }, { value: 'zulu', label: 'Azul Zulu' }, { value: 'mojang', label: 'Mojang' }], get: (s) => s.java.downloadVendor, set: (s, v) => (s.java.downloadVendor = v) } },
  { id: 'memory', tab: 'java', group: 'Java', title: 'Default memory', desc: (info) => (info?.memoryMb ? `${gb(info.memoryMb)} GB installed. ` : '') + 'Instances can override.', control: { kind: 'inputs', fields: [
    { label: 'Minimum', numeric: true, suffix: 'GB min', get: (s) => gb(s.java.memoryMinMb), set: (s, v) => (s.java.memoryMinMb = gbField(v)) },
    { label: 'Maximum', numeric: true, suffix: 'GB max', get: (s) => gb(s.java.memoryMaxMb), set: (s, v) => (s.java.memoryMaxMb = gbField(v)) },
  ] } },
  { id: 'jvmArgs', tab: 'java', group: 'Java', title: 'Default JVM arguments', control: { kind: 'textarea', rows: 2, get: (s) => s.java.jvmArgs, set: (s, v) => (s.java.jvmArgs = v) } },
  { id: 'warnLowMemory', tab: 'java', group: 'Java', title: 'Warn when free memory is low', desc: 'Checks before each launch.', control: { kind: 'toggle', get: (s) => s.java.warnLowMemory, set: (s, v) => (s.java.warnLowMemory = v) } },
  { id: 'suggestMemory', tab: 'java', group: 'Instances', title: 'Suggest memory from mod count', desc: 'Shown in each instance’s settings.', control: { kind: 'toggle', get: (s) => s.java.suggestMemory, set: (s, v) => (s.java.suggestMemory = v) } },

  { id: 'autoUpdate', tab: 'updates', group: 'Updates', title: 'Automatic launcher updates', desc: 'Downloads signed updates in the background and installs them on restart.', control: { kind: 'toggle', get: (s) => s.updates.autoCheck, set: (s, v) => (s.updates.autoCheck = v) } },
];

/** Whether a setting is currently shown (rows with `when` hide until it holds). */
export const visible = (d: SettingDef, s: Settings | null, info?: AppInfo) => !d.when || (!!s && d.when(s, info));

/** The settings of one tab, grouped by section heading in registry order. */
export function groupsFor(tab: SettingsTab): { group: string; defs: SettingDef[] }[] {
  const out: { group: string; defs: SettingDef[] }[] = [];
  for (const d of registry.filter((r) => r.tab === tab)) {
    const g = out.find((x) => x.group === d.group) ?? out[out.push({ group: d.group, defs: [] }) - 1]!;
    g.defs.push(d);
  }
  return out;
}

/** A setting's description text; built ones are searched by their fixed wording. */
export const descText = (d: SettingDef, info?: AppInfo) => (typeof d.desc === 'function' ? d.desc(info) : (d.desc ?? ''));

/** Case-insensitive search over titles and descriptions. */
export function search(q: string): SettingDef[] {
  const n = q.trim().toLowerCase();
  if (!n) return [];
  return registry.filter((d) => d.title.toLowerCase().includes(n) || descText(d).toLowerCase().includes(n) || d.group.toLowerCase().includes(n));
}

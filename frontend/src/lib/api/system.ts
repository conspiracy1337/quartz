// Real Go services. Each call falls back to a harmless browser-preview behaviour when
// the Wails bridge is absent, so the UI also runs under plain Vite and in tests.
import { bridged, hasBridge } from '../bridge';
import * as AppGo from '../wailsjs/go/api/App';
import * as SettingsGo from '../wailsjs/go/api/Settings';
import * as UpdatesGo from '../wailsjs/go/api/Updates';
import type { api as goapi, settings, theme, update } from '../wailsjs/go/models';
import { defaultSettings } from './mock/settings';

export type Settings = settings.Settings;
export type OpenTarget = goapi.OpenTarget;
export type UpdateState = update.State;
export type AppInfo = goapi.AppInfo;
export type CustomCSSState = theme.CSSState;

let previewSettings = defaultSettings();
let previewProxyPassword = false;

export const app = {
  info: (): Promise<AppInfo> =>
    hasBridge()
      ? AppGo.Info()
      : Promise.resolve({ name: 'Quartz Client', shortName: 'Quartz', version: 'preview', dataDir: '~/.local/share/quartz', portable: false, os: 'linux', memoryMb: 32768, discord: true, dataDirFixed: false }),
  /** Opens an https link in the system browser (Go refuses any other scheme). */
  openUrl: (url: string): Promise<void> => (hasBridge() ? AppGo.OpenURL(url) : (window.open(url, '_blank', 'noopener'), Promise.resolve())),
  /** Shows the launcher's data folder in the file manager. */
  openDataFolder: bridged(AppGo.OpenDataFolder, () => undefined),
  /** What the launcher was asked to open from outside since the last call (links, files, the command line). */
  pending: bridged(AppGo.Pending, (): OpenTarget[] => []),
  /** Asks for a new data folder; the launcher restarts to move there ('' when cancelled). */
  changeDataFolder: bridged(AppGo.ChangeDataFolder, () => ''),
  /** Closes the launcher even with downloads unfinished (they can be retried after the next start). */
  quit: bridged(AppGo.Quit, () => undefined),
};

// The browser preview shows the design's ready banner (Launcher 1.4.0).
const previewUpdate: UpdateState = { status: 'ready', current: 'preview', version: '1.4.0', date: '2026-10-01', notes: ['Faster start on large libraries', 'Shortcuts can open a world or server', 'Fixes for imports from other launchers'] };

/** The launcher's own updates. */
export const updatesApi = {
  state: bridged(UpdatesGo.State, () => previewUpdate),
  /** Looks for a newer release now; a newer one downloads in the background. */
  check: bridged(UpdatesGo.Check, () => previewUpdate),
  /** Installs the downloaded update and restarts into it. */
  restart: bridged(UpdatesGo.Restart, () => undefined),
};

export const settingsApi = {
  get: (): Promise<Settings> => (hasBridge() ? SettingsGo.Get() : Promise.resolve(structuredClone(previewSettings))),
  save: (s: Settings): Promise<Settings> => {
    if (hasBridge()) return SettingsGo.Save(s);
    previewSettings = structuredClone(s);
    return Promise.resolve(structuredClone(previewSettings));
  },
  pickCustomCSS: (): Promise<Settings> => (hasBridge() ? SettingsGo.PickCustomCSS() : Promise.resolve(structuredClone(previewSettings))),
  customCSS: (): Promise<CustomCSSState> => (hasBridge() ? SettingsGo.CustomCSS() : Promise.resolve({ path: '', css: '', error: '' })),
  exportTheme: (css: string): Promise<void> => (hasBridge() ? SettingsGo.ExportTheme(css) : Promise.resolve()),
  /** Stores the custom proxy password in the OS keychain; an empty string removes it. */
  setProxyPassword: (pw: string): Promise<void> => {
    if (hasBridge()) return SettingsGo.SetProxyPassword(pw);
    previewProxyPassword = pw !== '';
    return Promise.resolve();
  },
  hasProxyPassword: (): Promise<boolean> => (hasBridge() ? SettingsGo.HasProxyPassword() : Promise.resolve(previewProxyPassword)),
};

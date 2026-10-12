// Real Go services. Each call falls back to a harmless browser-preview behaviour when
// the Wails bridge is absent, so the UI also runs under plain Vite and in tests.
import { hasBridge } from '../bridge';
import * as AppGo from '../wailsjs/go/api/App';
import * as SettingsGo from '../wailsjs/go/api/Settings';
import type { api as goapi, settings, theme } from '../wailsjs/go/models';
import { defaultSettings } from './mock/settings';

export type Settings = settings.Settings;
export type AppInfo = goapi.AppInfo;
export type CustomCSSState = theme.CSSState;

let previewSettings = defaultSettings();
let previewProxyPassword = false;

export const app = {
  info: (): Promise<AppInfo> =>
    hasBridge()
      ? AppGo.Info()
      : Promise.resolve({ name: 'Quartz Client', shortName: 'Quartz', version: 'preview', dataDir: '~/.local/share/Quartz', portable: false, os: 'linux', memoryMb: 32768 }),
  /** Opens an https link in the system browser (Go refuses any other scheme). */
  openUrl: (url: string): Promise<void> => (hasBridge() ? AppGo.OpenURL(url) : (window.open(url, '_blank', 'noopener'), Promise.resolve())),
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

import type { settings } from '../../wailsjs/go/models';

/** Browser-preview copy of Go's settings.Defaults(). Only used when the Wails bridge is absent. */
export const defaultSettings = (): settings.Settings => ({
  general: { language: 'system', hideOnGameStart: false, startPage: 'home', startWithComputer: false, muteUnfocused: false, setupDone: true },
  downloads: { parallel: 8, watchDownloads: true, proxy: 'system', proxyCustom: { type: 'http', host: '', port: 8080, user: '' } },
  accounts: { autoRefresh: true, offlineFallback: true },
  integrations: { discordStatus: true, handleLinks: true },
  appearance: {
    theme: 'monolith', accent: '#3ecf8e', density: 'comfortable', textSize: 'default', reduceMotion: 'system',
    customCss: { enabled: false, path: '' },
  },
  java: {
    autoPick: true, downloadVendor: 'temurin', memoryMinMb: 1024, memoryMaxMb: 4096,
    jvmArgs: '-XX:+UseG1GC -XX:+ParallelRefProcEnabled', warnLowMemory: true, suggestMemory: true,
  },
  updates: { autoCheck: true, modChannel: 'release' },
  storage: { snapshots: true },
}) as settings.Settings;

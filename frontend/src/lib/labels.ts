// Display names for domain values, shared so every screen words them the same way.
import type { InstanceSummary, Loader, LoaderRef } from './api/types';
import type { SettingsTab } from './router.svelte';

export const LOADER_NAMES: Record<Loader, string> = { fabric: 'Fabric', quilt: 'Quilt', forge: 'Forge', neoforge: 'NeoForge' };

/** The loader's own product name, as its release notes say it: "Fabric Loader", "NeoForge". */
export const LOADER_PRODUCTS: Record<Loader, string> = { fabric: 'Fabric Loader', quilt: 'Quilt Loader', forge: 'Forge', neoforge: 'NeoForge' };

/** "Fabric 0.16.10" */
export const loaderLabel = (l: LoaderRef) => `${LOADER_NAMES[l.kind]} ${l.version}`;

/** "1.21.4, Fabric" or just "1.21.4" for vanilla. */
export const instanceMeta = (i: Pick<InstanceSummary, 'minecraft' | 'loader'>) =>
  i.loader ? `${i.minecraft}, ${LOADER_NAMES[i.loader.kind]}` : i.minecraft;

/** Settings tab names, for the tab strip and for "Settings, Downloads" hints. */
export const SETTINGS_TAB_LABELS: Record<SettingsTab, string> = {
  general: 'General', appearance: 'Appearance', java: 'Java', downloads: 'Downloads', storage: 'Storage',
  syncing: 'Syncing', integrations: 'Integrations', updates: 'Updates', about: 'About',
};

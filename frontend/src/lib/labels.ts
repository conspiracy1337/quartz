// Display names for domain values, shared so every screen words them the same way.
import type { ContentKind, InstanceSummary, Loader, LoaderRef, ProjectKind, ProjectType, Src } from './api/types';
import type { SettingsTab } from './router.svelte';

export const LOADER_NAMES: Record<Loader, string> = { fabric: 'Fabric', quilt: 'Quilt', forge: 'Forge', neoforge: 'NeoForge' };
/** A mod loader the launcher installs (not a shader loader, not vanilla). */
export const isModLoader = (l: string): l is Loader => l in LOADER_NAMES;

/** The loader's own product name, as its release notes say it: "Fabric Loader", "NeoForge". */
export const LOADER_PRODUCTS: Record<Loader, string> = { fabric: 'Fabric Loader', quilt: 'Quilt Loader', forge: 'Forge', neoforge: 'NeoForge' };

/** "Fabric 0.16.10" */
/** Every loader a version can name: mod loaders and shader loaders. */
export const ANY_LOADER_NAMES: Record<string, string> = { ...LOADER_NAMES, iris: 'Iris', optifine: 'OptiFine', canvas: 'Canvas' };
export const loaderLabel = (l: LoaderRef) => `${LOADER_NAMES[l.kind]} ${l.version}`;

/** "1.21.4, Fabric" or just "1.21.4" for vanilla. */
export const instanceMeta = (i: Pick<InstanceSummary, 'minecraft' | 'loader'>) =>
  i.loader ? `${i.minecraft}, ${LOADER_NAMES[i.loader.kind]}` : i.minecraft;

/** Where the system keeps sign-ins and passwords, as that system names it. */
export const keychainName = (os?: string) =>
  os === 'windows' ? 'Windows Credential Manager' : os === 'darwin' ? 'your login keychain' : 'your system keyring';

/** Settings tab names, for the tab strip and for "Settings, Downloads" hints. */
export const SETTINGS_TAB_LABELS: Record<SettingsTab, string> = {
  general: 'General', appearance: 'Appearance', java: 'Java', downloads: 'Downloads', storage: 'Storage',
  integrations: 'Integrations', updates: 'Updates', about: 'About',
};

/** Browse's tabs: Go's project type, its label, and the instance tab it installs into. */
export const PROJECT_TYPES: { kind: ProjectKind; label: ProjectType; content?: ContentKind; noun: string }[] = [
  { kind: 'mod', label: 'Mods', content: 'mods', noun: 'mod' },
  { kind: 'modpack', label: 'Modpacks', noun: 'modpack' },
  { kind: 'resourcepack', label: 'Resource packs', content: 'resourcepacks', noun: 'resource pack' },
  { kind: 'shader', label: 'Shaders', content: 'shaders', noun: 'shader pack' },
  { kind: 'datapack', label: 'Data packs', content: 'datapacks', noun: 'data pack' },
  { kind: 'world', label: 'Worlds', content: 'worlds', noun: 'world' },
];
export const projectType = (k: ProjectKind) => PROJECT_TYPES.find((t) => t.kind === k) ?? PROJECT_TYPES[0]!;
/** The project type a content tab installs. */
export const projectKindOf = (c: ContentKind): ProjectKind => PROJECT_TYPES.find((t) => t.content === c)?.kind ?? 'mod';

/** A version's channel as a badge: green release, orange beta, red alpha. */
export const CHANNEL_BADGE = { Release: 'badge-ok', Beta: 'badge-warn', Alpha: 'badge-danger' } as const;

export const SRC_NAMES: Record<Src, string> = { modrinth: 'Modrinth', curseforge: 'CurseForge', norisk: 'NoRisk' };

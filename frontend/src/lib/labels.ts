// Display names for domain values, shared so every screen words them the same way.
import type { InstanceSummary, Loader, LoaderRef } from './api/types';

export const LOADER_NAMES: Record<Loader, string> = { fabric: 'Fabric', quilt: 'Quilt', forge: 'Forge', neoforge: 'NeoForge' };

/** "Fabric 0.16.10" */
export const loaderLabel = (l: LoaderRef) => `${LOADER_NAMES[l.kind]} ${l.version}`;

/** "1.21.4, Fabric" or just "1.21.4" for vanilla. */
export const instanceMeta = (i: Pick<InstanceSummary, 'minecraft' | 'loader'>) =>
  i.loader ? `${i.minecraft}, ${LOADER_NAMES[i.loader.kind]}` : i.minecraft;

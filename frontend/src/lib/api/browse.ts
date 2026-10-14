// Browse, the project page, installs, modpack imports and updates, and blocked
// downloads (Go's browse, modpack and manual services). The preview serves the
// design's fixtures in the same shapes.
import { bridged } from '../bridge';
import { revs } from '../revs.svelte';
import * as BrowseGo from '../wailsjs/go/api/Browse';
import * as ContentGo from '../wailsjs/go/api/Content';
import * as ImportsGo from '../wailsjs/go/api/Imports';
import * as ManualGo from '../wailsjs/go/api/Manual';
import * as ModpacksGo from '../wailsjs/go/api/Modpacks';
import * as mock from './mock/browse';
import * as misc from './mock/misc';
import type {
  Art, BrowseQuery, BrowseResults, ContentKind, DiffKind, ImportPreview, InstallPlan, ManualView, ModpackDiff, PackUpdate, PackVersion,
  ProjectKind, ProjectPage, ProjectVersion, Src, VersionNotes,
} from './types';

const KINDS: Record<string, DiffKind> = { added: 'Added', updated: 'Updated', removed: 'Removed' };

export const browseApi = {
  /** Starts a search over both platforms; the first page of merged rows. */
  search: bridged(BrowseGo.Search, (q: BrowseQuery): BrowseResults => mock.search(q)),
  /** The next page: rows to append and rows a twin merged into. */
  more: bridged(BrowseGo.More, (_searchId: string): BrowseResults => ({ searchId: 'preview', rows: [], changed: [], more: false, failed: [] })),
  categories: bridged(BrowseGo.Categories, (kind: ProjectKind): string[] => mock.categories(kind)),
  /** A project page; instance (optional) is where Install goes. */
  project: (ref: string, instance: string) => revs.load('content', bridged(() => BrowseGo.Project(ref, instance), (): ProjectPage => mock.iris)),
  versions: (ref: string, src: Src, instance: string, compatible: boolean) =>
    revs.load('content', bridged(() => BrowseGo.Versions(ref, src, instance, compatible), (): ProjectVersion[] => mock.versions(compatible))),
  /** The project a description links to ("modrinth/mod/sodium"), as a reference for its page. */
  resolve: bridged(BrowseGo.Resolve, (_link: string) => 'mr:AANobbMI'),
  /** Changes of the given versions, or of the newest four. */
  changelog: bridged(BrowseGo.Changelog, (_ref: string, _src: Src, ids: string[]): VersionNotes[] => mock.changelog(ids)),
  /** RevC: the version to install into an instance and its dependencies ("" version for the newest that fits). */
  planInstall: bridged(ContentGo.PlanInstall,
    (_instance: string, _kind: ContentKind, _world: string, _ref: string, _src: Src, _version: string): InstallPlan => misc.installPlan),
  /** Starts the install job with the optional dependencies picked; resolves with its id. */
  applyInstall: bridged(ContentGo.ApplyInstall, (_planId: string, _optional: string[]) => ''),
};

export const importsApi = {
  inspectLink: bridged(ImportsGo.InspectLink, (_link: string): ImportPreview | null => misc.importPreview),
  /** Opens a file picker; null when cancelled. */
  inspectFile: bridged(ImportsGo.InspectFile, (): ImportPreview | null => misc.importPreview),
  inspectDropped: bridged(ImportsGo.InspectDropped, (_dropId: string): ImportPreview | null => misc.importPreview),
  /** A pack from Browse: the version picked on its project page, or "" for the newest release. */
  inspectProject: bridged(ImportsGo.InspectProject, (_ref: string, _src: Src, _version: string): ImportPreview | null => misc.importPreview),
  /** A pack the launcher was asked to open from outside (a link or a file), by its open id. */
  inspectOpened: bridged(ImportsGo.InspectOpened, (_openId: string): ImportPreview | null => misc.importPreview),
  /** Creates the instance as a job; resolves with its id. */
  install: bridged(ImportsGo.Install, (_token: string, _name: string, _art: Art, _enable: string[]) => ''),
};

type GoDiff = Omit<ModpackDiff, 'rows'> & { rows: { name: string; version: string; kind: string }[] | null };
const toDiff = (d: GoDiff): ModpackDiff => ({ ...d, keptEdits: d.keptEdits ?? [], rows: (d.rows ?? []).map((r) => ({ ...r, kind: KINDS[r.kind] ?? 'Updated' })) });

export const packsApi = {
  /** Packs with a newer version, from the last check. */
  updates: () => revs.load('modpacks', bridged(ModpacksGo.Updates, (): PackUpdate[] => [{ instanceId: 'cab', name: 'Create: Above and Beyond', version: '1.4.1' }])),
  /** The versions of an instance's pack, newest first. */
  versions: bridged(ModpacksGo.Versions, (_id: string): PackVersion[] => misc.packVersions),
  /** The review of moving to a version ("" for the one the last check found). */
  diff: bridged((id: string, version: string) => ModpacksGo.Diff(id, version).then((d) => toDiff(d as unknown as GoDiff)), (_id: string, _version: string): ModpackDiff => misc.modpackDiff),
  /** Starts the job moving to a version ("" for the one the last check found), backing worlds up first when asked; resolves with its id. */
  update: bridged(ModpacksGo.Update, (_id: string, _version: string, _backup: boolean) => ''),
};

export const manualApi = {
  view: (jobId: string) => revs.load('manual', bridged(() => ManualGo.View(jobId), (): ManualView => misc.manualView)),
  useModrinth: bridged(ManualGo.UseModrinth, (_jobId: string, _itemId: string) => undefined),
  skip: bridged(ManualGo.Skip, (_jobId: string) => undefined),
  addDropped: bridged(ManualGo.AddDropped, (_jobId: string, _dropId: string) => 1),
  openPages: bridged(ManualGo.OpenPages, (_jobId: string) => undefined),
};

// Actions on one instance that several screens offer (the header's menu and the
// instance settings), written once so they behave and word themselves the same.
import { api } from './api';
import type { ContentKind, Instance, InstallPlan, Src } from './api/types';

/** What the shared actions need to know about an instance. */
type Named = Pick<Instance, 'id' | 'name'>;
import { memoryShort } from './look';
import { prefs } from './prefs.svelte';
import { revs } from './revs.svelte';
import { go } from './router.svelte';
import { session } from './session.svelte';
import { tasks } from './tasks.svelte';

/** Re-checks the game files; a toast says so when nothing needed fixing. */
export async function repair(i: Named) {
  try {
    const job = await api.launch.repair(i.id);
    if (job) await tasks.load();
    else session.notify({ subject: i.name, text: 'All game files are intact.' });
  } catch (e) {
    session.fail('Repair', e);
  }
}

/** Moves the instance to the trash and offers Undo (it stays restorable for 30 days). */
export async function trash(i: Named) {
  try {
    const entry = await api.instances.trash(i.id);
    revs.bump('instances');
    go({ name: 'library' }, { replace: true });
    session.notify({
      tone: 'trash', subject: i.name, text: 'to the trash',
      undo: async () => {
        try {
          const back = await api.instances.restore(entry.key);
          revs.bump('instances');
          go({ name: 'instance', id: back.id, tab: 'overview' });
        } catch (e) {
          session.fail('Undo', e);
        }
      },
    });
  } catch (e) {
    session.fail('Delete', e);
  }
}

/** Copies the instance into a new one as a job; a toast with Open says when it is done. */
export async function duplicate(i: Named) {
  try {
    const job = await api.instances.duplicate(i.id);
    if (job) await tasks.load();
  } catch (e) {
    session.fail('Duplicate', e);
  }
}

/** Moves the game folder to a folder the user picks, as a download-style job. */
export async function move(i: Named, back = false) {
  try {
    const job = await (back ? api.instances.moveDefault(i.id) : api.instances.move(i.id));
    if (job) await tasks.load();
  } catch (e) {
    session.fail('Move', e);
  }
}

/**
 * Moves an instance to another Minecraft version or loader. With mods, Go first
 * moves them in a job (the instance changes when it finishes); the toast says what
 * happened to mods that couldn't come along.
 */
export async function switchVersion(i: Instance, minecraft: string, kind: string, build: string, label: string) {
  try {
    const v = await api.instances.setVersion(i.id, minecraft, kind, build);
    revs.bump('instances');
    const s = v.switch;
    if (s?.job) await tasks.load();
    const off = s?.disabled.length ?? 0;
    if (off > 0) session.notify({ subject: `${off} ${off === 1 ? 'mod was' : 'mods were'} turned off`, text: `no build for ${label}` });
    else if (s?.unknown.length) session.notify({ subject: label, text: `${s.unknown.length} unidentified ${s.unknown.length === 1 ? 'mod' : 'mods'} kept as they are` });
    go({ name: 'instance', id: i.id, tab: 'settings' });
    return true;
  } catch (e) {
    session.fail(label, e);
    return false;
  }
}

/** A started install keeps the page where it is and shows the download queue in a floating panel. */
export async function downloading(job: unknown) {
  if (!job) return;
  await tasks.load();
  session.downloadPanel = true;
}

/** Installs a project into an instance. A plan with nothing to decide downloads straight away;
    otherwise the Install dialog shows it (version, dependencies, memory). Data packs ask for a
    world first; a world has no plan, so it always downloads straight away. */
export async function installInto(instanceId: string, ref: string, source: Src, content: ContentKind, version = '') {
  const open = (plan?: InstallPlan) => session.open({ kind: 'install', ref, source, instanceId, content, ...(version ? { version } : {}), ...(plan ? { plan } : {}) });
  if (content === 'datapacks') return open();
  try {
    if (content === 'worlds') return await downloading(await api.browse.installWorld(instanceId, ref, version));
    const [plan, inst, info] = await Promise.all([api.browse.planInstall(instanceId, content, '', ref, source, version), api.instances.get(instanceId), api.app.info()]);
    if (!settled(plan, inst, info.memoryMb)) return open(plan);
    await downloading(await api.browse.applyInstall(plan.planId, []));
  } catch (e) {
    session.fail('Install', e);
  }
}

/** Nothing to show: a version on the update channel fits, it is a new item, it brings no
    dependency (an installed, up-to-date one isn't listed), and the memory hint would stay quiet. */
function settled(p: InstallPlan, inst: Instance, memoryMb: number) {
  const quiet = p.kind !== 'mods' || prefs.value?.java.suggestMemory === false || !memoryShort(p.mods + 1, memoryMb, inst.memoryMB.max);
  return !!p.version && !p.channel && !p.replaces && p.deps.length === 0 && quiet;
}

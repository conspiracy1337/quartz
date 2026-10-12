// Actions on one instance that several screens offer (the header's menu and the
// instance settings), written once so they behave and word themselves the same.
import { api } from './api';
import type { Instance } from './api/types';
import { revs } from './revs.svelte';
import { go } from './router.svelte';
import { session } from './session.svelte';
import { tasks } from './tasks.svelte';

/** Re-checks the game files; a toast says so when nothing needed fixing. */
export async function repair(i: Instance) {
  try {
    const job = await api.launch.repair(i.id);
    if (job) await tasks.load();
    else session.notify({ subject: i.name, text: 'All game files are intact.' });
  } catch (e) {
    session.fail('Repair', e);
  }
}

/** Moves the instance to the trash and offers Undo (it stays restorable for 30 days). */
export async function trash(i: Instance) {
  try {
    const entry = await api.instances.trash(i.id);
    revs.bump('instances');
    go({ name: 'library' });
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
    session.fail('Move to trash', e);
  }
}

/** Moves the game folder to a folder the user picks, as a download-style job. */
export async function move(i: Instance, back = false) {
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
    go({ name: 'instance', id: i.id, tab: 'version' });
    return true;
  } catch (e) {
    session.fail(label, e);
    return false;
  }
}

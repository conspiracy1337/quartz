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

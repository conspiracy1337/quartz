<script lang="ts">
  import { api } from '$lib/api';
  import type { TrashEntry } from '$lib/api/types';
  import { ago, bytes, plural } from '$lib/format';
  import { query, skeletonRows } from '$lib/query.svelte';
  import { revs } from '$lib/revs.svelte';
  import { go } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import ConfirmMenu, { Confirm } from '$lib/ui/ConfirmMenu.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Tile from '$lib/ui/Tile.svelte';
  import { groupsFor } from '$lib/settings/registry';
  import SettingsGroup from '$lib/settings/SettingsGroup.svelte';
  import SettingsTab from './SettingsTab.svelte';

  const info = query(() => api.app.info(), 'app.info');

  /** Deleting for good asks once more in a menu under the button. */
  const confirm = new Confirm();

  const usage = query(() => api.content.usage());
  let cleaning = $state(false);
  const unusedDesc = $derived(usage.data
    ? usage.data.unusedBytes > 0 ? `${bytes(usage.data.unusedBytes)} of mods and packs no instance uses any more.` : 'Mods and packs no instance uses any more. Nothing to clean right now.'
    : 'Mods and packs that no instance uses any more.');
  async function cleanUp() {
    cleaning = true;
    try {
      const before = usage.data?.unusedBytes ?? 0;
      const after = await api.content.cleanUp();
      revs.bump('content');
      session.notify({ subject: 'Cleaned up', text: `${bytes(Math.max(0, before - after.unusedBytes))} freed` });
    } catch (err) {
      session.fail('Clean up', err);
    } finally {
      cleaning = false;
    }
  }
  // Old logs of every instance (id "").
  const oldLogs = query(() => revs.load('instances', () => api.logs.oldSize('')));
  const oldDesc = $derived(oldLogs.data === undefined
    ? 'Compressed game logs older than 30 days.'
    : oldLogs.data > 0 ? `${bytes(oldLogs.data)} of compressed game logs older than 30 days.` : 'Compressed game logs older than 30 days. Nothing to clean right now.');
  async function deleteOld() {
    try {
      const freed = await api.logs.deleteOld('');
      revs.bump('instances');
      session.notify({ subject: 'Cleaned up', text: `${bytes(freed)} freed` });
    } catch (err) {
      session.fail('Delete old logs', err);
    }
  }
  // Screenshots still saved as PNG, re-saved as JPEG on request (a job in Downloads).
  const pngs = query(() => api.screenshots.pngs());
  const pngDesc = $derived(pngs.data?.count
    ? `${plural(pngs.data.count, 'screenshot')} saved as PNG, ${bytes(pngs.data.bytes)}. Saving them as JPEG keeps a fraction of that.`
    : 'Screenshots saved as PNG. Nothing to compress right now.');
  async function compressAll() {
    try {
      if (await api.screenshots.compressAll()) session.notify({ subject: 'Compressing screenshots', text: 'in Downloads' });
    } catch (err) {
      session.fail('Compress screenshots', err);
    }
  }
  // Keyed: the skeleton has as many rows as the trash had last time (an empty trash is one row).
  const trash = query(() => api.instances.trashList(), 'trash');
  const KEEP_DAYS = 30;
  const daysLeft = (e: TrashEntry) => Math.max(0, KEEP_DAYS - Math.floor((Date.now() - e.trashedAt) / 86_400_000));

  async function restore(e: TrashEntry) {
    try {
      const back = await api.instances.restore(e.key);
      revs.bump('instances');
      go({ name: 'instance', id: back.id, tab: 'overview' });
    } catch (err) {
      session.fail('Restore', err);
    }
  }
  async function purge(e: TrashEntry) {
    try {
      await api.instances.purge(e.key);
      revs.bump('instances');
      session.notify({ subject: e.name, text: 'was deleted' });
    } catch (err) {
      session.fail('Delete', err);
    }
  }
</script>

<SettingsTab tab="storage">
  <section class="grp" aria-label="Data folder">
    <h2 class="h3">Data folder</h2>
    <div class="card">
      <Setting title="Location" desc={info.data?.portable ? 'Portable mode: kept next to the launcher.' : 'Instances, the shared store and logs live here.'}><span class="mono small trunc muted" title={info.data?.dataDir}>{info.data?.dataDir ?? ''}</span></Setting>
      <div class="divider"></div>
      <Setting title="Open the data folder"><button class="btn btn-sm" onclick={() => api.app.openDataFolder().catch((e: unknown) => session.fail('Open', e))}>Open</button></Setting>
    </div>
  </section>
  <section class="grp" aria-label="Trash">
    <h2 class="h3">Trash</h2>
    <div class="list">
      {#each trash.data ?? [] as e (e.key)}
        <div class="row tr">
          <Tile art={e.art} icon={e.icon} loader={e.loader ?? ''} />
          <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{e.name}</span><span class="trunc small faint" style="display: block">Minecraft {e.minecraft}, moved to the trash {ago(e.trashedAt)}</span></span>
          <span class="small muted opt tnum">{plural(daysLeft(e), 'day')} left</span>
          <span style="display: flex; gap: 6px; justify-content: flex-end"><button class="btn btn-sm" onclick={() => restore(e)}>Restore</button><button class="btn btn-sm btn-danger" onclick={(ev) => confirm.ask(ev, `Delete ${e.name} for good? It can't be restored.`, 'Delete', () => purge(e))}>Delete now</button></span>
        </div>
      {:else}
        {#if trash.data}
          <div class="row empty tr-empty"><span>The trash is empty. Instances you move here can be restored for 30 days.</span></div>
        {:else}
          {#each { length: skeletonRows('trash', 1) } as _, k (k)}<div class="row tr" aria-hidden="true"><div class="sk" style="width: 32px; height: 32px; border-radius: 7px"></div><div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: 35%"></div><div class="sk" style="height: 10px; width: 50%"></div></div><div class="sk opt" style="height: 10px; width: 70px"></div><div></div></div>{/each}
        {/if}
      {/each}
    </div>
  </section>
  {#each groupsFor('storage') as g (g.group)}<SettingsGroup title={g.group} defs={g.defs} />{/each}
  <section class="grp" aria-label="Clean up">
    <h2 class="h3">Clean up</h2>
    <div class="card">
      <Setting title="Old logs" desc={oldDesc}><button class="btn btn-sm" disabled={!oldLogs.data} onclick={(ev) => confirm.ask(ev, `Delete ${bytes(oldLogs.data ?? 0)} of old logs for good?`, 'Delete', deleteOld)}>Delete old logs</button></Setting>
      <div class="divider"></div>
      <Setting title="Screenshots" desc={pngDesc}><button class="btn btn-sm" disabled={!pngs.data?.count} onclick={(ev) => confirm.ask(ev, `Save ${plural(pngs.data?.count ?? 0, 'screenshot')} as JPEG? The PNG files are replaced, at the quality set above.`, 'Compress', compressAll)}>Compress</button></Setting>
      <div class="divider"></div>
      <Setting title="Unused files in the shared store" desc={unusedDesc}><button class="btn btn-sm" disabled={cleaning || !usage.data?.unusedBytes} onclick={(ev) => confirm.ask(ev, `Delete ${bytes(usage.data?.unusedBytes ?? 0)} of files no instance uses? They download again if needed.`, 'Clean up', cleanUp)}>Clean up</button></Setting>
    </div>
  </section>
  <ConfirmMenu {confirm} />
</SettingsTab>


<style>
  .tr{grid-template-columns:32px minmax(0,1fr) 110px auto;height:60px}
  @media (max-width:1100px){.tr{grid-template-columns:32px minmax(0,1fr) auto}.tr .opt{display:none}}
  .tr-empty{height:60px;padding:0 var(--inset);flex-direction:row}
</style>

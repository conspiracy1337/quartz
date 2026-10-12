<script lang="ts">
  import { api } from '$lib/api';
  import type { TrashEntry } from '$lib/api/types';
  import { ago, bytes, plural } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import { revs } from '$lib/revs.svelte';
  import { go } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Tile from '$lib/ui/Tile.svelte';
  import SettingsTab from './SettingsTab.svelte';

  const info = query(() => api.app.info());
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
  const trash = query(() => api.instances.trashList());
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
      <Setting title="Open the data folder"><button class="btn btn-sm">Open</button></Setting>
    </div>
  </section>
  <section class="grp" aria-label="Trash">
    <h2 class="h3">Trash</h2>
    <div class="list">
      {#each trash.data ?? [] as e (e.key)}
        <div class="row tr">
          <Tile art={e.art} icon={e.icon} />
          <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{e.name}</span><span class="trunc small faint" style="display: block">Minecraft {e.minecraft}, moved to the trash {ago(e.trashedAt)}</span></span>
          <span class="small muted opt tnum">{plural(daysLeft(e), 'day')} left</span>
          <span style="display: flex; gap: 6px; justify-content: flex-end"><button class="btn btn-sm" onclick={() => restore(e)}>Restore</button><button class="btn btn-sm btn-danger" onclick={() => purge(e)}>Delete now</button></span>
        </div>
      {:else}
        {#if trash.data}
          <div class="row empty tr-empty"><span>The trash is empty. Instances you move here can be restored for 30 days.</span></div>
        {:else}
          {#each { length: 2 } as _, k (k)}<div class="row tr" aria-hidden="true"><div class="sk" style="width: 32px; height: 32px; border-radius: 7px"></div><div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: 35%"></div><div class="sk" style="height: 10px; width: 50%"></div></div><div class="sk opt" style="height: 10px; width: 70px"></div><div></div></div>{/each}
        {/if}
      {/each}
    </div>
  </section>
  <section class="grp" aria-label="Clean up">
    <h2 class="h3">Clean up</h2>
    <div class="card">
      <Setting title="Old logs" desc="Compressed game logs older than 30 days."><button class="btn btn-sm">Delete old logs</button></Setting>
      <div class="divider"></div>
      <Setting title="Unused files in the shared store" desc={unusedDesc}><button class="btn btn-sm" disabled={cleaning || !usage.data?.unusedBytes} onclick={cleanUp}>Clean up</button></Setting>
    </div>
  </section>
</SettingsTab>


<style>
  .tr{grid-template-columns:32px minmax(0,1fr) 110px auto;height:60px}
  @media (max-width:1100px){.tr{grid-template-columns:32px minmax(0,1fr) auto}.tr .opt{display:none}}
  .tr-empty{height:60px;padding:0 var(--inset);flex-direction:row}
</style>

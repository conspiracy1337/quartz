<script lang="ts">
  import { api } from '$lib/api';
  import type { SyncEntry } from '$lib/api/types';
  import { query } from '$lib/query.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import PageHeader from '$lib/ui/PageHeader.svelte';
  import ToggleRow from '$lib/ui/ToggleRow.svelte';

  const groups = query(() => api.sync.groups());
  const keys = query(() => api.sync.keybinds());
  let custom = $state('');

  const users = (e: SyncEntry, total: number) => (e.enabled && e.available ? `${e.users}/${total}` : '—');
  const sections = [
    { key: 'files', title: 'Files', note: 'Newest copy wins' },
    { key: 'folders', title: 'Folders', note: 'Shared with each instance' },
    { key: 'mods', title: 'Mod data', note: 'Greyed out mods are in no instance' },
  ] as const;
</script>

<PageHeader title="Syncing">
  {#snippet actions()}<button class="btn"><Icon name="folder" size={14} sw={2} />Open synced folder</button>{/snippet}
</PageHeader>
<div class="scroll">
  <div class="page" style="gap: 24px; padding-top: 18px">
    <p class="muted" style="margin: 0; max-width: 80ch">Turn on what can be shared here, then pick which instances join in their own settings. Folders are shared with each instance; files keep the newest copy. The number shows how many instances use each one.</p>
    {#if groups.data}
      {@const g = groups.data}
      <div class="page-split">
        <div>
          {#each sections as sec (sec.key)}
            <section class="sec" aria-label={sec.title}>
              <div class="sec-h"><h2 class="h3">{sec.title}</h2><span class="small faint">{sec.note}</span></div>
              <div class="card tgl-grid">
                {#each g[sec.key] as e (e.id)}
                  <ToggleRow name={e.name} hint={e.path} count={users(e, g.total)} bind:on={e.enabled} disabled={!e.available} title={e.available ? undefined : 'Not installed in any instance'} />
                {/each}
              </div>
            </section>
          {/each}
        </div>
        <div>
          <section class="sec" aria-label="Custom">
            <div class="sec-h"><h2 class="h3">Custom</h2><span class="small faint">Paths inside the instance folder</span></div>
            <div class="card">
              <div class="tgl-grid">
                {#each g.custom as e (e.id)}
                  <ToggleRow name="" hint={e.path} pathOnly count={users(e, g.total)} bind:on={e.enabled}>
                    {#snippet after()}<button class="btn btn-icon btn-sm btn-ghost" aria-label="Remove {e.path}"><Icon name="x" size={10} sw={2.2} /></button>{/snippet}
                  </ToggleRow>
                {/each}
              </div>
              <div class="divider"></div>
              <div style="display: flex; gap: 8px; align-items: center; padding: 12px var(--inset)">
                <label class="input" style="flex: 1 1 auto"><span class="sr">Path inside the instance folder</span><input class="mono" placeholder="config/journeymap/" bind:value={custom} /></label>
                <button class="btn">Add file</button>
                <button class="btn">Add folder</button>
              </div>
            </div>
          </section>
          <section class="sec" aria-label="Shared keybinds">
            <div class="sec-h"><h2 class="h3">Shared keybinds <span class="count">{keys.data?.length ?? ''}</span></h2><button class="btn btn-sm">Add key</button></div>
            <div class="list">
              {#each keys.data ?? [] as k (k.name)}
                <div class="row krow"><span class="trunc">{k.name}</span><button class="btn btn-sm" style="justify-content: space-between; font-family: var(--mono)">{k.key}</button><button class="btn btn-icon btn-sm btn-ghost" aria-label="Stop sharing {k.name}"><Icon name="x" size={10} sw={2.2} /></button></div>
              {/each}
            </div>
            <p class="small faint" style="margin: 0">Only these keys are written into options.txt on launch. Keys a version doesn't have are skipped.</p>
          </section>
        </div>
      </div>
    {:else}
      <div class="page-split" aria-hidden="true"><div><div class="sk" style="height: 120px; border-radius: var(--radius-lg)"></div><div class="sk" style="height: 160px; border-radius: var(--radius-lg)"></div></div><div><div class="sk" style="height: 140px; border-radius: var(--radius-lg)"></div></div></div>
    {/if}
  </div>
</div>

<style>
  .krow{grid-template-columns:minmax(0,1fr) 120px 28px;height:44px}
</style>

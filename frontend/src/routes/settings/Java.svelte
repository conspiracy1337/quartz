<script lang="ts">
  import { api } from '$lib/api';
  import { plural } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import { groupsFor } from '$lib/settings/registry';
  import SettingsGroup from '$lib/settings/SettingsGroup.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import SettingsTab from './SettingsTab.svelte';

  const runtimes = query(() => api.java.runtimes());
  const defs = groupsFor('java').flatMap((g) => g.defs);
</script>

<SettingsTab tab="java" gap={20}>
  <section style="display: flex; flex-direction: column; gap: 8px" aria-label="Runtimes">
    <div style="display: flex; justify-content: space-between; align-items: center"><h2 class="h3">Java runtimes</h2><div style="display: flex; gap: 6px"><button class="btn btn-sm">Scan again</button><button class="btn btn-sm">Add from folder</button><button class="btn btn-sm btn-accent">Download</button></div></div>
    <div class="list">
      {#each runtimes.data ?? [] as j (j.path)}
        <div class="row jr">
          <span class="maj">{j.major}</span>
          <span style="min-width: 0"><span style="display: flex; gap: 8px; align-items: center"><span class="trunc" style="font-weight: 500">{j.name}</span><span class="badge" class:badge-accent={j.source === 'Managed'}>{j.source}</span></span><span class="trunc small faint mono" style="display: block">{j.path}</span></span>
          <span class="small muted opt">{j.usedBy ? plural(j.usedBy, 'instance') : 'Not used'}</span>
          <button class="btn btn-icon btn-sm btn-ghost" aria-label="More for {j.name}"><Icon name="more" size={14} /></button>
        </div>
      {:else}
        {#each { length: 6 } as _, k (k)}<div class="row jr" aria-hidden="true"><div class="sk" style="width: 44px; height: 32px; border-radius: 6px"></div><div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: 35%"></div><div class="sk" style="height: 10px; width: 50%"></div></div><div class="sk opt" style="height: 10px; width: 70px"></div><div></div></div>{/each}
      {/each}
    </div>
  </section>
  <SettingsGroup {defs} />
</SettingsTab>

<style>
  .jr{grid-template-columns:44px minmax(0,1fr) 110px 28px;height:60px}
  @media (max-width:1100px){.jr{grid-template-columns:44px minmax(0,1fr) 28px}.jr .opt{display:none}}
  .maj{width:44px;height:32px;border-radius:6px;background:var(--raise);display:grid;place-items:center;font-family:var(--mono);font-weight:600}
</style>

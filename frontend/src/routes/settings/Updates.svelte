<script lang="ts">
  import { api } from '$lib/api';
  import { query } from '$lib/query.svelte';
  import { groupsFor } from '$lib/settings/registry';
  import SettingsGroup from '$lib/settings/SettingsGroup.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import SettingsTab from './SettingsTab.svelte';

  const info = query(() => api.app.info());
</script>

<SettingsTab tab="updates">
  <section class="grp" aria-label="Launcher version">
    <h2 class="h3">Launcher</h2>
    <div class="card">
      <Setting title="Version {info.data?.version ?? ''}" desc="Updates are signed and verified before they install."><button class="btn btn-sm">Check now</button></Setting>
    </div>
  </section>
  {#each groupsFor('updates') as g (g.group)}<SettingsGroup title="Automatic updates" defs={g.defs} />{/each}
</SettingsTab>


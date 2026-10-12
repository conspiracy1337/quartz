<script lang="ts">
  import { api } from '$lib/api';
  import { query } from '$lib/query.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import SettingsTab from './SettingsTab.svelte';

  const info = query(() => api.app.info());
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
  <section class="grp" aria-label="Clean up">
    <h2 class="h3">Clean up</h2>
    <div class="card">
      <Setting title="Old logs" desc="Compressed game logs older than 30 days."><button class="btn btn-sm">Delete old logs</button></Setting>
      <div class="divider"></div>
      <Setting title="Unused files in the shared store" desc="Mods, libraries and assets that no instance uses any more."><button class="btn btn-sm">Clean up</button></Setting>
    </div>
  </section>
</SettingsTab>


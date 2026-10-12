<script lang="ts">
  import { api } from '$lib/api';
  import { query } from '$lib/query.svelte';
  import SettingsTab from './SettingsTab.svelte';

  const info = query(() => api.app.info());
</script>

<SettingsTab tab="about">
  <section class="grp" aria-label="About">
    <h2 class="h3">{info.data?.name ?? ''}</h2>
    <div class="card card-pad">
      <div class="kv">
        <span>Version</span><span class="mono">{info.data?.version ?? ''}</span>
        <span>System</span><span>{info.data?.os ?? ''}</span>
        <span>Data folder</span><span class="mono trunc muted">{info.data?.dataDir ?? ''}</span>
        <span>Telemetry</span><span>None. The launcher never sends usage data.</span>
      </div>
    </div>
  </section>
</SettingsTab>


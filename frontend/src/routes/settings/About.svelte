<script lang="ts">
  import { api } from '$lib/api';
  import { hours, plural } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import SettingsTab from './SettingsTab.svelte';

  const info = query(() => api.app.info(), 'app.info');
  const playtime = query(() => Promise.all([api.instances.totalPlaytime(), api.instances.list()]));
</script>

<SettingsTab tab="about">
  <section class="grp" aria-label="About">
    <h2 class="h3">{info.data?.name ?? ''}</h2>
    <div class="card card-pad">
      <div class="kv">
        <span>Version</span><span class="mono">{info.data?.version ?? ''}</span>
        <span>System</span><span>{info.data?.os ?? ''}</span>
        <span>Data folder</span><span class="mono trunc muted">{info.data?.dataDir ?? ''}{info.data?.portable ? ' (portable)' : ''}</span>
        <span>Playtime</span><span class="tnum">{playtime.data ? `${hours(playtime.data[0])} across ${plural(playtime.data[1].length, 'instance')}` : ''}</span>
        <span>Telemetry</span><span>None. The launcher never sends usage data.</span>
      </div>
    </div>
  </section>
</SettingsTab>


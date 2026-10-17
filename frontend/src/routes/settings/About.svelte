<script lang="ts">
  import { api } from '$lib/api';
  import { hours, plural } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import SettingsTab from './SettingsTab.svelte';

  const info = query(() => api.app.info(), 'app.info');
  const playtime = query(() => Promise.all([api.instances.totalPlaytime(), api.instances.list()]));
</script>

<!-- A value, or a skeleton holding its line until it is in. -->
{#snippet value(text: string | undefined, width: number)}{#if text === undefined}<span class="sk sk-text" style="width: {width}px" aria-hidden="true"></span>{:else}{text}{/if}{/snippet}

<SettingsTab tab="about">
  <section class="grp" aria-label="About">
    <h2 class="h3">{@render value(info.data?.name, 110)}</h2>
    <div class="card card-pad">
      <div class="kv">
        <span>Version</span><span class="mono">{@render value(info.data?.version, 72)}</span>
        <span>System</span><span>{@render value(info.data?.os, 140)}</span>
        <span>Data folder</span><span class="mono trunc muted">{@render value(info.data && info.data.dataDir + (info.data.portable ? ' (portable)' : ''), 260)}</span>
        <span>Playtime</span><span class="tnum">{@render value(playtime.data && `${hours(playtime.data[0])} across ${plural(playtime.data[1].length, 'instance')}`, 200)}</span>
        <span>Telemetry</span><span>None. The launcher never sends usage data.</span>
      </div>
    </div>
  </section>
</SettingsTab>


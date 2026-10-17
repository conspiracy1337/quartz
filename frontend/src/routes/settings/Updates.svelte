<script lang="ts">
  import { api } from '$lib/api';
  import { since } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import { session } from '$lib/session.svelte';
  import { groupsFor } from '$lib/settings/registry';
  import SettingsGroup from '$lib/settings/SettingsGroup.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import SettingsTab from './SettingsTab.svelte';

  const info = query(() => api.app.info(), 'app.info');
  const u = $derived(session.update);
  // The row's description says where the updater is (composed text).
  const status = $derived.by(() => {
    switch (u?.status) {
      case 'off': return 'Updates are off in this build.';
      case 'checking': return 'Checking for updates…';
      case 'downloading': return `Downloading version ${u.version}…`;
      case 'ready': return `Version ${u.version} is ready. It installs when you restart.`;
      case 'error': return `Couldn't check: ${u.error ?? ''}`;
    }
    if (u?.checked) return `Up to date. Checked ${since(u.checked)}.`;
    return 'Updates are signed and verified before they install.';
  });
  let busy = $state(false);
  async function check() {
    busy = true;
    try {
      session.update = await api.updates.check();
    } catch (e) {
      session.fail('Check for updates', e);
    } finally {
      busy = false;
    }
  }
</script>

<SettingsTab tab="updates">
  <section class="grp" aria-label="Launcher version">
    <h2 class="h3">Launcher</h2>
    <div class="card">
      <Setting title="Version {info.data?.version ?? ''}" desc={status}><button class="btn btn-sm" onclick={check} disabled={busy || !u || u.status === 'off' || u.status === 'checking' || u.status === 'downloading'}>Check now</button></Setting>
    </div>
  </section>
  {#each groupsFor('updates') as g (g.group)}<SettingsGroup title="Automatic updates" defs={g.defs} />{/each}
</SettingsTab>

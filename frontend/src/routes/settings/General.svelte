<script lang="ts">
  import { groupsFor } from '$lib/settings/registry';
  import SettingsGroup from '$lib/settings/SettingsGroup.svelte';
  import SettingsTab from './SettingsTab.svelte';

  // General shows the launcher basics plus the download and integration settings, as designed.
  const groups = [...groupsFor('general'), ...groupsFor('downloads').filter((g) => g.group === 'Downloads'), ...groupsFor('integrations')];
</script>

<SettingsTab tab="general">
  {#each groups as g (g.group)}<SettingsGroup title={g.group} defs={g.defs} />{/each}
</SettingsTab>

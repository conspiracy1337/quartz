<script lang="ts">
  import type { SettingsTab as Tab } from '$lib/router.svelte';
  import SettingsTab from './SettingsTab.svelte';

  /** A tab whose content is exactly its registry groups (Downloads, Integrations). */
  interface Props { tab: Tab }
  let { tab }: Props = $props();
</script>

<SettingsTab {tab} />

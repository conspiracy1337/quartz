<script lang="ts">
  import type { Snippet } from 'svelte';
  import SettingsHeader from '$lib/components/SettingsHeader.svelte';
  import type { SettingsTab } from '$lib/router.svelte';
  import { groupsFor } from '$lib/settings/registry';
  import SettingsGroup from '$lib/settings/SettingsGroup.svelte';

  /** A settings tab: header, then registry groups (or custom content) in balanced columns. */
  interface Props { tab: SettingsTab; gap?: number; children?: Snippet }
  let { tab, gap = 22, children }: Props = $props();
</script>

<SettingsHeader active={tab} />
<div class="scroll">
  <div class="page page-cols set-grid" style="gap: {gap}px; padding-top: 18px">
    {#if children}
      {@render children()}
    {:else}
      {#each groupsFor(tab) as g (g.group)}<SettingsGroup title={g.group} defs={g.defs} />{/each}
    {/if}
  </div>
</div>

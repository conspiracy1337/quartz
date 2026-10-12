<script lang="ts">
  import SettingsHeader from '$lib/components/SettingsHeader.svelte';
  import { plural } from '$lib/format';
  import { search } from '$lib/settings/registry';
  import SettingsGroup from '$lib/settings/SettingsGroup.svelte';

  interface Props { query: string }
  let { query }: Props = $props();
  const results = $derived(search(query));
</script>

<SettingsHeader {query} />
<div class="scroll">
  <div class="page set-grid" style="gap: 18px">
    <section aria-label="Results" style="display: flex; flex-direction: column; gap: 8px">
      {#if query.trim()}
        <span class="small faint">{results.length ? `${plural(results.length, 'setting')} match` : 'No settings match'}</span>
        {#if results.length}<SettingsGroup defs={results} mark={query} />{/if}
      {:else}
        <span class="small faint">Type to search every setting.</span>
      {/if}
    </section>
  </div>
</div>

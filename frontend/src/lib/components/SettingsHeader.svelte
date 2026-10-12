<script lang="ts">
  import { SETTINGS_TAB_LABELS } from '../labels';
  import { go, href, type SettingsTab } from '../router.svelte';
  import Icon from '../ui/Icon.svelte';
  import PageHeader from '../ui/PageHeader.svelte';
  import Tabs from '../ui/Tabs.svelte';

  /** `query` is set while searching; the tab strip then shows no active tab. */
  interface Props { active?: SettingsTab; query?: string }
  let { active, query }: Props = $props();

  const labels = SETTINGS_TAB_LABELS;
  // Syncing is one page in the sidebar; its settings tab opens that page instead of a copy.
  const items = (Object.keys(labels) as SettingsTab[]).map((k) => ({
    key: k, label: labels[k], href: k === 'syncing' ? href({ name: 'syncing' }) : href({ name: 'settings', tab: k }),
  }));
  const leave = () => go({ name: 'settings', tab: 'general' });
</script>

<PageHeader title="Settings">
  {#snippet actions()}
    {#if query === undefined}
      <a href={href({ name: 'settings-search', query: '' })} class="input" style="width: 240px; text-decoration: none"><Icon name="search" size={14} sw={1.8} /><span>Search settings</span></a>
    {:else}
      <label class="input" style="width: 240px">
        <Icon name="search" size={14} sw={1.8} /><span class="sr">Search settings</span>
        <!-- svelte-ignore a11y_autofocus -->
        <input type="search" value={query} autofocus
          oninput={(e) => go({ name: 'settings-search', query: e.currentTarget.value }, { replace: true })}
          onkeydown={(e) => e.key === 'Escape' && leave()} />
        <a href={href({ name: 'settings', tab: 'general' })} class="kbd" style="text-decoration: none" aria-label="Clear search">Esc</a>
      </label>
    {/if}
  {/snippet}
  {#snippet tabs()}
    <Tabs {items} {active} label="Settings sections" />
  {/snippet}
</PageHeader>

<script lang="ts">
  import { api } from '$lib/api';
  import type { ContentKind } from '$lib/api/types';
  import ContentTab from '$lib/components/ContentTab.svelte';
  import InstHeader from '$lib/components/InstHeader.svelte';
  import { query } from '$lib/query.svelte';
  import type { InstanceTab } from '$lib/router.svelte';
  import ChangeLoader from './ChangeLoader.svelte';
  import ChangeMinecraft from './ChangeMinecraft.svelte';
  import Commands from './Commands.svelte';
  import InstanceSettings from './InstanceSettings.svelte';
  import LogFiles from './LogFiles.svelte';
  import LogHistory from './LogHistory.svelte';
  import LogsLive from './LogsLive.svelte';
  import ModpackUpdate from './ModpackUpdate.svelte';
  import ModUpdates from './ModUpdates.svelte';
  import Overview from './Overview.svelte';
  import Sandbox from './Sandbox.svelte';
  import ScreenshotViewer from './ScreenshotViewer.svelte';
  import Screenshots from './Screenshots.svelte';
  import Version from './Version.svelte';

  interface Props { id: string; tab: InstanceTab; sub?: string | undefined }
  let { id, tab, sub }: Props = $props();

  const CONTENT: InstanceTab[] = ['mods', 'resourcepacks', 'shaders', 'datapacks', 'worlds', 'servers'];
  const instance = query(() => api.instances.get(id));
  const i = $derived(instance.data?.id === id ? instance.data : undefined);
  // Review screens replace the instance header with their own top bar.
  const review = $derived((tab === 'mods' && sub === 'updates') || (tab === 'overview' && sub === 'update'));
</script>

{#if i}
  {#if review}
    {#if tab === 'mods'}<ModUpdates instance={i} />{:else}<ModpackUpdate instance={i} />{/if}
  {:else}
    <InstHeader instance={i} {tab} />
    {#if tab === 'overview'}
      <Overview instance={i} />
    {:else if CONTENT.includes(tab)}
      <div class="scroll">{#key tab}<ContentTab instanceId={i.id} kind={tab as ContentKind} />{/key}</div>
    {:else if tab === 'screenshots'}
      {#if sub}<ScreenshotViewer instanceId={i.id} shotId={sub} />{:else}<Screenshots instanceId={i.id} />{/if}
    {:else if tab === 'logs'}
      {#if sub === 'files'}<LogFiles instanceId={i.id} />{:else if sub === 'history'}<LogHistory instanceId={i.id} />{:else}<LogsLive instanceId={i.id} />{/if}
    {:else if tab === 'version'}
      {#if sub === 'minecraft'}<ChangeMinecraft instance={i} />{:else if sub === 'loader'}<ChangeLoader instance={i} />{:else}<Version instance={i} />{/if}
    {:else if tab === 'settings'}
      {#if sub === 'commands'}<Commands instance={i} />{:else if sub === 'sandbox'}<Sandbox instance={i} />{:else}<InstanceSettings instance={i} />{/if}
    {/if}
  {/if}
{:else}
  <header style="padding: 16px 28px 0" aria-hidden="true">
    <div class="sk" style="height: 12px; width: 120px; margin-top: 4px"></div>
    <div style="display: grid; grid-template-columns: 56px minmax(0,1fr); column-gap: 16px; align-items: center; height: 56px; margin-top: 18px"><div class="sk" style="width: 56px; height: 56px; border-radius: 10px"></div><div style="display: flex; flex-direction: column; gap: 8px"><div class="sk" style="height: 20px; width: 260px"></div><div class="sk" style="height: 12px; width: 200px"></div></div></div>
    <div style="height: 40px; margin-top: 18px; border-bottom: 1px solid var(--line)"></div>
  </header>
{/if}

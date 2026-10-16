<script lang="ts">
  import { api } from '$lib/api';
  import { managed } from '$lib/api/content';
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

  interface Props { id: string; tab: InstanceTab; sub?: string | undefined }
  let { id, tab, sub }: Props = $props();

  const CONTENT: InstanceTab[] = ['mods', 'resourcepacks', 'shaders', 'datapacks', 'worlds', 'servers'];
  const instance = query(() => api.instances.get(id));
  const i = $derived(instance.data?.id === id ? instance.data : undefined);
  // Review screens replace the instance header with their own top bar.
  const review = $derived((managed(tab as ContentKind) && CONTENT.includes(tab) && sub === 'updates') || (tab === 'overview' && sub === 'update'));
  // Browse adds to the instance whose overview was opened last.
  $effect(() => void (tab === 'overview' && api.instances.opened(id).catch(() => {})));
  // Files changed outside the launcher show up while the instance is open.
  $effect(() => {
    void api.content.watch(id);
    return () => void api.content.watch('');
  });
</script>

{#if i}
  {#if review}
    {#if tab === 'overview'}<ModpackUpdate instance={i} />{:else}<ModUpdates instance={i} kind={tab as ContentKind} />{/if}
  {:else}
    <InstHeader instance={i} {tab} />
    {#if tab === 'overview'}
      <Overview instance={i} />
    {:else if CONTENT.includes(tab)}
      <div class="scroll">{#key tab}<ContentTab instanceId={i.id} kind={tab as ContentKind} chip={tab === 'mods' && sub === 'client-only' ? 'Client only' : undefined} />{/key}</div>
    {:else if tab === 'screenshots'}
      {#if sub}<ScreenshotViewer instanceId={i.id} shotId={sub} />{:else}<Screenshots instanceId={i.id} />{/if}
    {:else if tab === 'logs'}
      {#if sub === 'files'}<LogFiles instanceId={i.id} />{:else if sub === 'history'}<LogHistory instanceId={i.id} />{:else}<LogsLive instanceId={i.id} />{/if}
    {:else if tab === 'settings'}
      {#if sub === 'minecraft'}<ChangeMinecraft instance={i} />{:else if sub === 'loader'}<ChangeLoader instance={i} />{:else if sub === 'commands'}<Commands instance={i} />{:else if sub === 'sandbox'}<Sandbox instance={i} />{:else}<InstanceSettings instance={i} />{/if}
    {/if}
  {/if}
{:else}
  <header style="padding: 16px 28px 0" aria-hidden="true">
    <div class="sk" style="height: 12px; width: 120px; margin-top: 4px"></div>
    <div style="display: grid; grid-template-columns: 56px minmax(0,1fr); column-gap: 16px; align-items: center; height: 56px; margin-top: 18px"><div class="sk" style="width: 56px; height: 56px; border-radius: 10px"></div><div style="display: flex; flex-direction: column; gap: 8px"><div class="sk" style="height: 20px; width: 260px"></div><div class="sk" style="height: 12px; width: 200px"></div></div></div>
    <div style="height: 40px; margin-top: 18px; border-bottom: 1px solid var(--line)"></div>
  </header>
{/if}

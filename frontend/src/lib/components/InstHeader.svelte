<script lang="ts">
  import { api } from '../api';
  import type { Instance } from '../api/types';
  import { hours } from '../format';
  import { href, type InstanceTab } from '../router.svelte';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';
  import Tabs from '../ui/Tabs.svelte';
  import { loaderLabel } from '../labels';

  interface Props { instance: Instance; tab: InstanceTab }
  let { instance: i, tab }: Props = $props();

  const running = $derived(!!session.running[i.id]);
  const hosting = $derived(session.hosting[i.id] === 'running');
  const to = (t: InstanceTab) => href({ name: 'instance', id: i.id, tab: t });
  const tabs = $derived([
    { key: 'overview', label: 'Overview', href: to('overview') },
    { key: 'mods', label: 'Mods', count: i.counts.mods, href: to('mods') },
    { key: 'resourcepacks', label: 'Resource packs', count: i.counts.resourcepacks, href: to('resourcepacks') },
    { key: 'shaders', label: 'Shaders', count: i.counts.shaders, href: to('shaders') },
    { key: 'datapacks', label: 'Data packs', count: i.counts.datapacks, href: to('datapacks') },
    { key: 'worlds', label: 'Worlds', count: i.counts.worlds, href: to('worlds') },
    { key: 'servers', label: 'Servers', count: i.counts.servers, href: to('servers') },
    { key: 'screenshots', label: 'Screenshots', href: to('screenshots') },
    { key: 'logs', label: 'Logs', href: to('logs') },
    { key: 'version', label: 'Version', href: to('version') },
    { key: 'settings', label: 'Settings', href: to('settings') },
  ] satisfies { key: InstanceTab; label: string; count?: number; href: string }[]);
</script>

<header style="font-family: var(--font); color: var(--text); padding: 16px 28px 0; background: var(--bg)">
  <div class="crumbs"><a href={href({ name: 'library' })}>Library</a><Icon name="chevron-right" size={12} sw={2} /><span class="muted">{i.name}</span></div>
  <div style="display: grid; grid-template-columns: 56px minmax(0,1fr) auto; align-items: center; column-gap: 16px; height: 56px; margin-top: 14px">
    <div class="ico" style="width: 56px; height: 56px; border-radius: 10px; background: {i.art.background}; font-size: 22px" aria-hidden="true">
      {#if i.icon}<Icon name={i.icon} size={24} sw={1.8} />{:else}{i.art.initial}{/if}
    </div>
    <div style="min-width: 0; display: flex; flex-direction: column; gap: 4px">
      <h1 class="h1 trunc">{i.name}</h1>
      <div class="muted" style="display: flex; gap: 14px; height: 20px; align-items: center; overflow: hidden; white-space: nowrap">
        <span>Minecraft {i.minecraft}</span>
        {#if i.loader}<span>{loaderLabel(i.loader)}</span>{/if}
        <span class="tnum">{hours(i.playtimeMin)} played</span>
        {#if i.sandboxed}<span class="badge badge-info"><Icon name="shield" size={10} sw={2.4} />Sandboxed</span>{/if}
      </div>
    </div>
    <div style="display: flex; gap: 8px; align-items: center">
      <button class="btn btn-icon" aria-label="Open folder" onclick={() => api.instances.openFolder(i.id)}><Icon name="folder" /></button>
      <button class="btn btn-icon" aria-label="More actions"><Icon name="more" /></button>
      {#if hosting}
        <span class="btn" style="height: 40px; border-color: color-mix(in srgb, var(--accent) 45%, transparent); cursor: default" title="Server running"><span class="dot live" aria-hidden="true"></span><span class="mono" style="font-size: 12px">localhost:25565</span></span>
      {:else}
        <button class="btn" style="height: 40px" title="Run this instance as a server on your computer" onclick={() => session.open({ kind: 'host', instanceId: i.id })}><Icon name="server" />Host</button>
      {/if}
      {#if running}
        <button class="btn btn-stop" onclick={() => session.stop(i.id)}><Icon name="stop" size={12} />Stop</button>
      {:else}
        <button class="btn btn-play" onclick={() => session.play(i.id)}><Icon name="play-wide" size={14} />Play</button>
      {/if}
    </div>
  </div>
  <Tabs items={tabs} active={tab} label="Instance" style="margin-top: 18px" />
</header>

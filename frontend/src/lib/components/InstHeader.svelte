<script lang="ts">
  import { api } from '../api';
  import type { Instance } from '../api/types';
  import { hours } from '../format';
  import { href, type InstanceTab } from '../router.svelte';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';
  import InstanceMark from '../ui/InstanceMark.svelte';
  import Tabs from '../ui/Tabs.svelte';
  import { below } from '../ui/Menu.svelte';
  import { loaderLabel } from '../labels';

  interface Props { instance: Instance; tab: InstanceTab }
  let { instance: i, tab }: Props = $props();

  const running = $derived(session.busy(i.id));
  // ⋮ opens the same menu as a right-click on the instance.
  const menuOpen = $derived(session.instanceMenu?.instanceId === i.id);
  const host = $derived(session.hosting?.instanceId === i.id ? session.hosting : null);
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
    { key: 'settings', label: 'Settings', href: to('settings') },
  ] satisfies { key: InstanceTab; label: string; count?: number; href: string }[]);
</script>

<header style="font-family: var(--font); color: var(--text); padding: 16px 28px 0; background: var(--bg)">
  <div class="crumbs"><a href={href({ name: 'library' })}>Library</a><Icon name="chevron-right" size={12} sw={2} /><span class="muted">{i.name}</span></div>
  <div style="display: grid; grid-template-columns: 56px minmax(0,1fr) auto; align-items: center; column-gap: 16px; height: 56px; margin-top: 14px">
    <div class="ico" style="width: 56px; height: 56px; border-radius: 10px; background: {i.art.background}; font-size: 22px" aria-hidden="true">
      <InstanceMark art={i.art} icon={i.icon} />
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
      <!-- Host lives in the ⋮ menu; while the server runs its address shows here and toggles its panel. -->
      {#if host?.state === 'running'}
        <button class="btn" style="height: 40px; border-color: color-mix(in srgb, var(--accent) 45%, transparent)" title="Server running" aria-pressed={session.hostPanel} onclick={() => (session.hostPanel = !session.hostPanel)}><span class="dot live" aria-hidden="true"></span><span class="mono" style="font-size: 12px">{host.address}</span></button>
      {/if}
      <button class="btn-bare" aria-label="Open folder" title="Open folder" onclick={() => api.instances.openFolder(i.id)}><Icon name="folder" size={18} /></button>
      {#if running}
        <button class="btn btn-stop" onclick={() => session.stop(i.id)}><Icon name="stop" size={12} />Stop</button>
      {:else}
        <button class="btn btn-play" onclick={() => session.play(i.id)}><Icon name="play-wide" size={14} />Play</button>
      {/if}
      <button class="btn btn-icon" style="height: 40px; width: 40px" aria-label="More actions" aria-haspopup="menu" aria-expanded={menuOpen} onclick={(e) => (session.instanceMenu = { instanceId: i.id, ...below(e) })}><Icon name="more" /></button>
    </div>
  </div>
  <Tabs items={tabs} active={tab} label="Instance" style="margin-top: 18px" />
</header>


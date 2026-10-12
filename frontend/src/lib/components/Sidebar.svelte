<script lang="ts" module>
  export type SideKey = 'home' | 'library' | 'browse' | 'account' | 'sync' | 'downloads' | 'settings' | 'instance';
</script>

<script lang="ts">
  import { api } from '../api';
  import { avatar } from '../look';
  import { revs } from '../revs.svelte';
  import { query } from '../query.svelte';
  import { href, type Route } from '../router.svelte';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';
  import type { IconName } from '../ui/icons';
  import Tile from '../ui/Tile.svelte';

  interface Props { active: SideKey; instanceId?: string | undefined; downloads?: number }
  let { active, instanceId, downloads = 0 }: Props = $props();

  const recent = query(() => api.instances.recent());
  const all = query(() => api.instances.list());
  const accounts = query(() => api.accounts.list());
  const me = $derived(accounts.data?.find((a) => a.isDefault));

  const nav: { key: SideKey; label: string; icon: IconName; route: Route }[] = [
    { key: 'home', label: 'Home', icon: 'home', route: { name: 'home' } },
    { key: 'library', label: 'Library', icon: 'library', route: { name: 'library' } },
    { key: 'browse', label: 'Browse', icon: 'compass', route: { name: 'browse' } },
    { key: 'account', label: 'Skins', icon: 'shirt', route: { name: 'skins' } },
    { key: 'sync', label: 'Syncing', icon: 'sync', route: { name: 'syncing' } },
  ];
</script>

<aside class="side" style="font-family: var(--font); color: var(--text)">
  <div class="side-brand">
    <Icon name="brand" size={18} />
    <span class="hide-c">Quartz Client</span>
  </div>
  <button class="side-link" style="border: 1px solid var(--line-2); background: var(--panel); color: var(--faint); text-align: left" onclick={() => session.open({ kind: 'palette', mode: 'commands' })}>
    <Icon name="search" size={14} sw={1.8} />
    <span>Search</span><span class="kbd">Ctrl K</span>
  </button>
  <nav class="side-nav" aria-label="Main">
    {#each nav as n (n.key)}
      <a href={href(n.route)} class="side-link" class:on={active === n.key} aria-current={active === n.key ? 'page' : undefined}>
        <Icon name={n.icon} /><span>{n.label}</span>
        {#if n.key === 'library'}<span class="count">{all.data?.length ?? ''}</span>{:else}<span></span>{/if}
      </a>
    {/each}
  </nav>
  <div style="display: flex; flex-direction: column; gap: 2px; min-height: 0">
    <div class="side-label hide-c"><span>Recent</span><a href={href({ name: 'library' })} style="color: var(--faint); text-decoration: none">All</a></div>
    {#if !recent.data}
      {#each { length: 4 } as _, k (k)}
        <div class="side-inst" aria-hidden="true"><div class="sk" style="width: 20px; height: 20px"></div><div class="sk" style="height: 10px; width: {[70, 80, 55, 65][k]}%"></div><span></span></div>
      {/each}
    {/if}
    {#each recent.data ?? [] as i (i.id)}
      <a href={href({ name: 'instance', id: i.id, tab: 'overview' })} class="side-inst" class:on={active === 'instance' && instanceId === i.id} oncontextmenu={(e) => session.openLaunchAs(i.id, e)}>
        <Tile art={i.art} icon={i.icon} size="sm" />
        <span class="trunc">{i.name}</span>
        {#if session.running[i.id]}<span class="dot live" aria-label="Running"></span>{:else}<span></span>{/if}
      </a>
    {/each}
  </div>
  <div class="side-foot">
    <a href={href({ name: 'downloads' })} class="side-link" class:on={active === 'downloads'}>
      <Icon name="download" /><span>Downloads</span>
      {#if downloads > 0}<span class="badge badge-accent">{downloads}</span>{:else}<span></span>{/if}
    </a>
    <a href={href({ name: 'settings', tab: 'general' })} class="side-link" class:on={active === 'settings'}>
      <Icon name="settings" /><span>Settings</span><span></span>
    </a>
    <div class="divider" style="margin: 6px 0"></div>
    <a href={href({ name: 'accounts' })} class="side-acct" style="text-decoration: none; color: var(--text)">
      {#if me}
        <span class="avatar" aria-hidden="true" style="background: {avatar(me, 28)}; image-rendering: pixelated"></span>
        <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{me.name}</span><span class="trunc small faint" style="display: block">{me.kind === 'microsoft' ? 'Microsoft account' : 'Offline account'}</span></span>
      {:else if accounts.data}
        <span class="avatar" aria-hidden="true" style="background: var(--raise)"></span>
        <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">Add account</span><span class="trunc small faint" style="display: block">No account yet</span></span>
      {:else}
        <span class="avatar sk" aria-hidden="true"></span>
        <span style="min-width: 0; display: flex; flex-direction: column; gap: 6px" aria-hidden="true"><span class="sk" style="display: block; height: 11px; width: 70%"></span><span class="sk" style="display: block; height: 9px; width: 85%"></span></span>
      {/if}
      <Icon name="up-down" size={14} sw={2} style="color: var(--faint)" />
    </a>
  </div>
</aside>

<style>
  /* .side .hide-c (display:block) would otherwise beat .side-label's flex row. Only while
     the sidebar is expanded; collapsed, monolith.css hides it. */
  @media (min-width:1101px){.side-label.hide-c{display:flex}}
</style>

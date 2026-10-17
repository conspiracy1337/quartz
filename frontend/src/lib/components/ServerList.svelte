<script lang="ts">
  import { api } from '../api';
  import type { MergedServer } from '../api/types';
  import { players } from '../format';
  import { query, skeletonRows } from '../query.svelte';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';
  import ServerPing from '../ui/ServerPing.svelte';
  import ServerVersion from '../ui/ServerVersion.svelte';
  import SplitButton from '../ui/SplitButton.svelte';
  import Tile from '../ui/Tile.svelte';
  import Empty from '../ui/Empty.svelte';

  /** Servers from every instance, de-duplicated. Join uses the instance last used on it. */
  const ROW_H = 52;
  const HEAD_H = 44;

  let reload = $state(0);
  // Refresh pings every server again; other reloads use the last minute's answers.
  let force = false;
  const servers = query(() => {
    void reload; // re-fetch when Refresh bumps it
    const f = force;
    force = false;
    return api.home.servers(f);
  }, 'home.servers'); // opening Home again shows the last answers while it pings
  function refresh() {
    force = true;
    reload++;
  }
  const instances = query(() => api.instances.list(), 'instances.list');
  let onlineOnly = $state(false);
  let open = $state<string | null>(null);

  const rows = $derived((servers.data ?? []).filter((s) => !onlineOnly || s.online));
  const openIndex = $derived(rows.findIndex((r) => r.id === open));
  const menuFor = $derived(rows[openIndex]);

  const inst = (id: string) => instances.data?.find((i) => i.id === id);

  /** Joins the server with an instance (the one last used on it by default). */
  function join(s: MergedServer, instanceId: string | undefined) {
    open = null;
    if (instanceId) session.play(instanceId, '', { kind: 'server', target: s.address });
  }
  function filter(online: boolean) {
    onlineOnly = online;
    open = null;
  }
</script>

<section class="card sl" aria-label="Servers" style="font-family: var(--font); color: var(--text)">
  <div class="card-head" style="flex: 0 0 44px">
    <h2 class="h3">Servers <span class="count">{servers.data?.length ?? ''}</span></h2>
    <span style="display: flex; gap: 6px; align-items: center">
      <button class="chip" class:on={!onlineOnly} onclick={() => filter(false)}>All</button>
      <button class="chip" class:on={onlineOnly} onclick={() => filter(true)}>Online</button>
      <button class="btn-bare" aria-label="Refresh" title="Refresh" onclick={refresh}><Icon name="refresh" size={14} sw={1.8} /></button>
    </span>
  </div>
  <div class="sl-body">
    {#if servers.data}
      {#each rows as s (s.id)}
        <div class="srow">
          <span class="fav" style="background: {s.art.background}" aria-hidden="true"></span>
          <span class="s-id"><span class="s-name trunc">{s.name}</span><span class="s-ip trunc">{s.address}</span></span>
          <span class="c-ver"><ServerVersion server={s} /></span>
          <span class="c-play small tnum trunc" style="color: {s.online ? 'var(--text)' : 'var(--faint)'}">{s.online ? players(s.players) : 'No reply'}</span>
          <ServerPing ms={s.pingMs} />
          <SplitButton size="sm" accent={s.online} style="justify-self: end" onclick={() => join(s, s.instances[0])}
            onmenu={() => (open = open === s.id ? null : s.id)} expanded={open === s.id} menuLabel="Join {s.name} with another instance">Join</SplitButton>
        </div>
      {:else}
        <Empty text={servers.data.length ? 'No servers are online right now.' : 'Servers from all your instances show up here.'} />
      {/each}
    {:else}
      {#each { length: skeletonRows('home.servers', 6) } as _, k (k)}
        <div class="srow" aria-hidden="true">
          <div class="sk" style="width: 32px; height: 32px; border-radius: 6px"></div>
          <div class="sk" style="height: 12px; width: {[40, 30, 46, 36, 32, 50][k % 6]}%"></div>
          <div class="sk c-ver" style="height: 20px; width: 56px"></div>
          <div class="sk c-play" style="height: 10px; width: 48px"></div>
          <div class="sk" style="height: 10px; width: 64px"></div>
          <div class="sk" style="height: 28px; width: 72px; justify-self: end"></div>
        </div>
      {/each}
    {/if}
  </div>
  {#if menuFor}
    <div class="menu jm" role="menu" aria-label="Join with" style="top: {HEAD_H + (openIndex + 1) * ROW_H - 4}px">
      <div class="small faint menu-title">Join {menuFor.name} with</div>
      {#each menuFor.instances as id, j (id)}
        {@const i = inst(id)}
        {#if i}
          <button class="mi" role="menuitem" onclick={() => join(menuFor, id)}><Tile art={i.art} icon={i.icon} size="sm" /><span class="trunc">{i.name}</span><span class="small faint">{j === 0 ? 'Last used' : i.minecraft}</span></button>
        {/if}
      {/each}
    </div>
  {/if}
</section>

<style>
  .sl{position:absolute;inset:0;display:flex;flex-direction:column;container-type:inline-size;overflow:hidden}
  .sl-body{flex:1 1 auto;min-height:0;overflow:auto}
  .srow{display:grid;grid-template-columns:32px minmax(0,1fr) 104px 88px 84px 88px;align-items:center;column-gap:16px;height:52px;padding:0 var(--inset);border-top:1px solid var(--line)}
  .srow:first-child{border-top:0}
  .srow:hover{background:color-mix(in srgb,var(--raise) 60%,transparent)}
  @container (max-width:640px){.srow{grid-template-columns:32px minmax(0,1fr) 88px 84px 88px}.srow .c-ver{display:none}}
  @container (max-width:480px){.srow{grid-template-columns:32px minmax(0,1fr) 84px 88px}.srow .c-play{display:none}}
  /* Name and address share one line; the address gives way first when space runs out. */
  .s-id{display:flex;align-items:baseline;gap:10px;min-width:0}
  .s-name{font-weight:500;flex:0 1 auto}
  .s-ip{flex:0 100 auto;font-family:var(--mono);font-size:12px;color:var(--faint)}
  .fav{width:32px;height:32px;border-radius:6px}
  .jm{position:absolute;right:var(--inset);z-index:5;min-width:240px}
</style>

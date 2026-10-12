<script lang="ts">
  import { api } from '../api';
  import type { MergedServer } from '../api/types';
  import { count } from '../format';
  import { query } from '../query.svelte';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';
  import Tile from '../ui/Tile.svelte';
  import Empty from '../ui/Empty.svelte';

  /** Servers from every instance, de-duplicated. Join uses the instance last used on it. */
  const ROW_H = 52;
  const HEAD_H = 44;

  let reload = $state(0);
  const servers = query(() => {
    void reload; // re-fetch when Refresh bumps it
    return api.home.servers();
  });
  const instances = query(() => api.instances.list());
  let onlineOnly = $state(false);
  let open = $state<string | null>(null);

  const rows = $derived((servers.data ?? []).filter((s) => !onlineOnly || s.online));
  const openIndex = $derived(rows.findIndex((r) => r.id === open));
  const menuFor = $derived(rows[openIndex]);

  const players = (s: MergedServer) => (s.players ? (s.players.max ? `${s.players.online} / ${s.players.max}` : count(s.players.online)) : 'No reply');
  const pingClass = (ms?: number) => (ms === undefined ? '' : ms < 30 ? 'p4' : ms < 80 ? 'p3' : ms < 250 ? 'p2' : 'p1');
  const inst = (id: string) => instances.data?.find((i) => i.id === id);

  function join(instanceId: string | undefined) {
    open = null;
    if (instanceId) session.play(instanceId);
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
      <button class="btn btn-icon btn-sm btn-ghost" aria-label="Refresh" onclick={() => reload++}><Icon name="refresh" size={14} sw={1.8} /></button>
    </span>
  </div>
  <div class="sl-body">
    {#if servers.data}
      {#each rows as s (s.id)}
        <div class="srow">
          <span class="fav" style="background: {s.art.background}" aria-hidden="true"></span>
          <span class="s-id"><span class="s-name trunc">{s.name}</span><span class="s-ip trunc">{s.address}</span></span>
          <span class="c-ver"><span class="badge" class:badge-danger={!s.online}>{s.version}</span></span>
          <span class="c-play small tnum trunc" style="color: {s.online ? 'var(--text)' : 'var(--faint)'}">{players(s)}</span>
          <span class="s-ping"><span class="s-ms">{s.pingMs !== undefined ? `${s.pingMs} ms` : ''}</span><span class="ping {pingClass(s.pingMs)}" aria-hidden="true"><i style="height: 4px"></i><i style="height: 6px"></i><i style="height: 8px"></i><i style="height: 11px"></i></span></span>
          <span class="split">
            <button class="btn btn-sm" class:btn-accent={s.online} onclick={() => join(s.instances[0])}>Join</button>
            <button class="btn btn-sm" onclick={() => (open = open === s.id ? null : s.id)} aria-expanded={open === s.id} aria-label="Join {s.name} with another instance"><Icon name="chevron-down" size={10} sw={2.4} /></button>
          </span>
        </div>
      {:else}
        <Empty text={servers.data.length ? 'No servers are online right now.' : 'Servers from all your instances show up here.'} />
      {/each}
    {:else}
      {#each { length: 6 } as _, k (k)}
        <div class="srow" aria-hidden="true">
          <div class="sk" style="width: 32px; height: 32px; border-radius: 6px"></div>
          <div class="sk" style="height: 12px; width: {[40, 30, 46, 36, 32, 50][k]}%"></div>
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
      <div class="small faint" style="padding: 6px 8px 4px">Join {menuFor.name} with</div>
      {#each menuFor.instances as id, j (id)}
        {@const i = inst(id)}
        {#if i}
          <button class="mi" role="menuitem" onclick={() => join(id)}><Tile art={i.art} size="sm" /><span class="trunc">{i.name}</span><span class="small faint">{j === 0 ? 'Last used' : i.minecraft}</span></button>
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
  .s-ping{display:flex;align-items:center;gap:8px}
  .s-ms{width:46px;text-align:right;font-family:var(--mono);font-size:12px;color:var(--muted);font-variant-numeric:tabular-nums}
  .fav{width:32px;height:32px;border-radius:6px}
  .ping{display:inline-flex;align-items:flex-end;gap:2px;height:11px}
  .ping i{width:3px;background:var(--line-2);border-radius:1px}
  .ping.p4 i,.ping.p3 i:nth-child(-n+3),.ping.p2 i:nth-child(-n+2),.ping.p1 i:nth-child(1){background:var(--accent)}
  .split{display:inline-flex;justify-self:end}
  .split .btn:first-child{border-radius:6px 0 0 6px}
  .split .btn:last-child{border-radius:0 6px 6px 0;border-left:0;width:24px;padding:0}
  .jm{position:absolute;right:var(--inset);z-index:5;min-width:240px}
  .mi{display:grid;grid-template-columns:20px minmax(0,1fr) auto;gap:10px;align-items:center;height:34px;padding:0 8px;border-radius:4px;color:var(--text);text-decoration:none;border:0;background:transparent;width:100%;text-align:left}
  .mi:hover{background:var(--hover)}
</style>

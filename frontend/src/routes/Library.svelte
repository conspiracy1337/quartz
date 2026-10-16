<script lang="ts">
  import InstanceMark from '$lib/ui/InstanceMark.svelte';
  import { api } from '$lib/api';
  import type { InstanceSummary } from '$lib/api/types';
  import { agoShort, hours, searchIn } from '$lib/format';
  import { instanceMeta } from '$lib/labels';
  import { query } from '$lib/query.svelte';
  import { href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import SearchField from '$lib/ui/SearchField.svelte';
  import Select from '$lib/ui/Select.svelte';
  import Empty from '$lib/ui/Empty.svelte';

  type Filter = 'all' | 'modded' | 'modpack' | 'vanilla' | 'servers';
  type Sort = 'last' | 'name' | 'playtime';
  const FILTERS: { key: Filter; label: string }[] = [
    { key: 'all', label: 'All' }, { key: 'modded', label: 'Modded' }, { key: 'modpack', label: 'Modpacks' }, { key: 'vanilla', label: 'Vanilla' }, { key: 'servers', label: 'Servers' },
  ];
  const SORTS = [{ value: 'last', label: 'Last played' }, { value: 'name', label: 'Name' }, { value: 'playtime', label: 'Playtime' }] as const;
  const SKELETON_WIDTHS = ['70%', '55%', '80%', '60%', '65%', '72%', '50%', '68%', '58%', '76%', '62%', '54%'];

  const list = query(() => api.instances.list());
  let search = $state('');
  let filter = $state<Filter>('all');
  let sort = $state<Sort>('last');

  const matches = (i: InstanceSummary) =>
    filter === 'all' || (filter === 'servers' ? i.hasServers : filter === 'modded' ? i.kind !== 'vanilla' : i.kind === filter);
  // "Pack 2" before "Pack 10"; an instance never played counts from when it was made.
  const names = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });
  const order: Record<Sort, (a: InstanceSummary, b: InstanceSummary) => number> = {
    last: (a, b) => (b.lastPlayed ?? b.created) - (a.lastPlayed ?? a.created),
    name: (a, b) => names.compare(a.name, b.name),
    playtime: (a, b) => b.playtimeMin - a.playtimeMin || names.compare(a.name, b.name),
  };
  const rows = $derived(
    (list.data ?? []).filter((i) => matches(i) && [i.name, ...i.tags].some((t) => t.toLowerCase().includes(search.trim().toLowerCase()))).sort(order[sort]),
  );
</script>

<div class="scroll">
  <div class="page">
    <div style="display: flex; align-items: center; gap: 8px; height: 32px">
      <h1 class="h1" style="margin-right: auto">Library</h1>
      <!-- As wide as the Sort select below, so both right-aligned controls line up. -->
      <a href={href({ name: 'new-instance' })} class="btn btn-accent" style="width: 150px"><Icon name="plus" size={14} sw={2} />New instance</a>
    </div>
    <div style="display: flex; flex-wrap: wrap; align-items: center; gap: 8px">
      <SearchField bind:value={search} label="Search instances" placeholder={searchIn('instances', list.data?.length)} style="flex: 0 1 260px" />
      <div style="display: flex; gap: 6px; flex-wrap: wrap">
        {#each FILTERS as f (f.key)}<button class="chip" class:on={filter === f.key} onclick={() => (filter = f.key)}>{f.label}</button>{/each}
      </div>
      <div style="flex: 1 1 0"></div>
      <Select bind:value={sort} options={SORTS} label="Sort instances" style="width: 150px" />
    </div>
    <div class="lgrid">
      {#if list.data}
        {#each rows as r (r.id)}
          <div class="icard" role="group" aria-label={r.name} oncontextmenu={(e) => session.openLaunchAs(r.id, e)}>
            <div class="icard-cover" style="background: radial-gradient(120% 140% at 0% 0%, {r.art.background} 0%, #141417 85%); opacity: .9"></div>
            <div style="padding: 0 14px 12px; display: flex; flex-direction: column; justify-content: space-between; min-width: 0">
              <span class="ico ico-lg" style="background: {r.art.background}; margin-top: -24px; box-shadow: 0 0 0 3px var(--panel); position: relative; z-index: 1"><InstanceMark art={r.art} icon={r.icon} /></span>
              <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{r.name}</span><span class="trunc small muted" style="display: block">{instanceMeta(r)}</span></span>
              <span class="small faint tnum" style="display: flex; justify-content: space-between"><span>{agoShort(r.lastPlayed)}</span><span>{hours(r.playtimeMin)}</span></span>
            </div>
            <a class="icard-hit" href={href({ name: 'instance', id: r.id, tab: 'overview' })} aria-label="Open {r.name}"></a>
            {#if session.running[r.id]}
              <button class="btn btn-stop btn-icon play live" style="width: 40px; padding: 0; border-radius: 20px" aria-label="Stop {r.name}" onclick={() => session.stop(r.id)}><Icon name="stop" size={12} /></button>
            {:else}
              <button class="btn btn-play btn-icon play" style="width: 40px; padding: 0; border-radius: 20px" aria-label="Play {r.name}" onclick={() => session.play(r.id)}><Icon name="play" size={14} /></button>
            {/if}
          </div>
        {:else}
          {#if list.data.length}
            <Empty text="No instances match this filter." style="grid-column: 1 / -1" />
          {:else}
            <Empty text="No instances yet. Create one to start playing." action="New instance" href={href({ name: 'new-instance' })} style="grid-column: 1 / -1" />
          {/if}
        {/each}
      {:else}
        {#each SKELETON_WIDTHS as w, k (k)}
          <div class="icard" aria-hidden="true">
            <div class="sk" style="border-radius: 0"></div>
            <div style="padding: 0 14px 12px; display: flex; flex-direction: column; justify-content: space-between">
              <div class="sk" style="width: 48px; height: 48px; border-radius: 9px; margin-top: -24px; box-shadow: 0 0 0 3px var(--panel); position: relative; z-index: 1"></div>
              <div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: {w}"></div><div class="sk" style="height: 10px; width: 45%"></div></div>
              <div class="sk" style="height: 10px; width: 30%"></div>
            </div>
          </div>
        {/each}
      {/if}
    </div>
  </div>
</div>

<style>
  .lgrid{display:grid;grid-template-columns:repeat(auto-fill,minmax(196px,1fr));gap:14px}
  .icard{height:184px;border:1px solid var(--line);border-radius:10px;background:var(--panel);overflow:hidden;display:grid;grid-template-rows:72px auto;text-decoration:none;color:var(--text);position:relative;transition:border-color 120ms ease}
  .icard:hover{border-color:var(--line-2)}
  /* The cover is translucent, which lifts it into its own layer; the icon sits above it explicitly. */
  .icard-cover{min-height:0}
  /* The whole card opens the instance; Play sits above it. */
  .icard-hit{position:absolute;inset:0;z-index:2;border-radius:10px}
  .icard-hit:focus-visible{outline:2px solid var(--accent);outline-offset:-2px}
  .icard .play{z-index:3;position:absolute;right:10px;top:52px;opacity:0;transform:translateY(4px);transition:opacity 140ms ease,transform 160ms var(--ease)}
  .icard:hover .play,.icard:focus-within .play,.icard .play.live{opacity:1;transform:none}
</style>

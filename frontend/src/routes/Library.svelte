<script lang="ts">
  import InstanceCard from '$lib/components/InstanceCard.svelte';
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

  const list = query(() => api.instances.list(), 'instances.list');
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
    <div style="display: flex; align-items: center; height: 32px">
      <h1 class="h1">Library</h1>
    </div>
    <div style="display: flex; flex-wrap: wrap; align-items: center; gap: 8px">
      <SearchField bind:value={search} label="Search instances" placeholder={searchIn('instances', list.data?.length)} style="flex: 0 1 260px" />
      <div style="display: flex; gap: 6px; flex-wrap: wrap">
        {#each FILTERS as f (f.key)}<button class="chip" class:on={filter === f.key} onclick={() => (filter = f.key)}>{f.label}</button>{/each}
      </div>
      <!-- Sort and New instance wrap as one group, so they always end the row together. -->
      <div style="display: flex; gap: 8px; margin-left: auto">
        <Select bind:value={sort} options={SORTS} label="Sort instances" style="width: 150px" />
        <a href={href({ name: 'new-instance' })} class="btn btn-accent"><Icon name="plus" size={14} sw={2} />New instance</a>
      </div>
    </div>
    <div class="lgrid">
      {#if list.data}
        {#each rows as r (r.id)}
          <InstanceCard art={r.art} icon={r.icon} loader={r.loader?.kind ?? ''} banner={r.banner} name={r.name} meta={instanceMeta(r)} left={agoShort(r.lastPlayed)} right={hours(r.playtimeMin)} oncontextmenu={(e) => session.openInstanceMenu(r.id, e)}>
            <a class="icard-hit" href={href({ name: 'instance', id: r.id, tab: 'overview' })} aria-label="Open {r.name}"></a>
            {#if session.busy(r.id)}
              <button class="btn btn-stop btn-icon play live" style="width: 40px; padding: 0; border-radius: 20px" aria-label="Stop {r.name}" onclick={() => session.stop(r.id)}><Icon name="stop" size={12} /></button>
            {:else}
              <button class="btn btn-play btn-icon play" style="width: 40px; padding: 0; border-radius: 20px" aria-label="Play {r.name}" onclick={() => session.play(r.id)}><Icon name="play" size={14} /></button>
            {/if}
          </InstanceCard>
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
  /* The whole card opens the instance; Play sits above it. */
  .icard-hit{position:absolute;inset:0;z-index:2;border-radius:10px}
  .icard-hit:focus-visible{outline:2px solid var(--accent);outline-offset:-2px}
  :global(.icard) .play{z-index:3;position:absolute;right:10px;top:52px;opacity:0;transform:translateY(4px);transition:opacity 140ms ease,transform 160ms var(--ease)}
  :global(.icard):hover .play,:global(.icard):focus-within .play,:global(.icard) .play.live{opacity:1;transform:none}
</style>

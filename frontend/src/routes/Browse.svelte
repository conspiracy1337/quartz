<script lang="ts">
  import { api } from '$lib/api';
  import type { ProjectHit, ProjectType } from '$lib/api/types';
  import { ago, compact } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import { href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Check from '$lib/ui/Check.svelte';
  import PageHeader from '$lib/ui/PageHeader.svelte';
  import SearchField from '$lib/ui/SearchField.svelte';
  import Select from '$lib/ui/Select.svelte';
  import Tabs from '$lib/ui/Tabs.svelte';
  import Tile from '$lib/ui/Tile.svelte';
  import Empty from '$lib/ui/Empty.svelte';

  const TYPES: ProjectType[] = ['Mods', 'Modpacks', 'Resource packs', 'Shaders', 'Data packs'];
  const SORTS = [{ value: 'relevance', label: 'Relevance' }, { value: 'downloads', label: 'Downloads' }, { value: 'updated', label: 'Recently updated' }] as const;
  const SKELETON: [string, string][] = [['30%', '60%'], ['24%', '50%'], ['36%', '70%'], ['28%', '40%'], ['32%', '64%'], ['40%', '56%'], ['26%', '48%']];

  let type = $state<ProjectType>('Mods');
  let search = $state('');
  let sort = $state<'relevance' | 'downloads' | 'updated'>('relevance');
  let cats = $state<Record<string, boolean>>({ 0: true });
  let hide = $state({ ai: true, ads: false, installed: true });

  const catalog = query(() => api.browse.catalog(type));
  const target = query(() => api.instances.first());
  const showTarget = $derived(type !== 'Modpacks');

  // Results merge both sources: downloads are added up, and the date is the most recent update on either side.
  const merged = (p: ProjectHit) => {
    const srcs = [p.modrinth, p.curseforge].filter((x) => x !== undefined);
    return { downloads: srcs.reduce((n, x) => n + x.downloads, 0), updated: Math.max(...srcs.map((x) => x.updated)) };
  };
  const results = $derived(
    (catalog.data?.hits ?? [])
      .filter((p) => p.name.toLowerCase().includes(search.trim().toLowerCase()))
      .map((p) => ({ ...p, ...merged(p) }))
      .sort((a, b) => (sort === 'downloads' ? b.downloads - a.downloads : sort === 'updated' ? b.updated - a.updated : 0)),
  );

  function pickType(t: ProjectType) {
    type = t;
    cats = {};
  }
</script>

<PageHeader title="Browse" style="padding-bottom: 16px; border-bottom: 1px solid var(--line)">
  {#snippet actions()}
    {#if showTarget && target.data}
      {@const t = target.data}
      <span class="small faint">Adding to</span>
      <Select value={t.id} options={[{ value: t.id, label: t.name }]} label="Instance to add to" style="width: 200px">
        {#snippet lead()}<Tile art={t.art} size="sm" />{/snippet}
      </Select>
    {/if}
  {/snippet}
</PageHeader>
<div class="bw">
  <section class="scroll" aria-label="Results">
    <div class="page" style="gap: 12px; padding-top: 4px">
      <Tabs items={TYPES.map((t) => ({ key: t, label: t }))} active={type} label="Content type" onselect={pickType} />
      <div style="display: flex; gap: 8px">
        <SearchField bind:value={search} label="Search" placeholder="Search {type.toLowerCase()}" style="flex: 1 1 auto" height={36} />
        <button class="btn fbtn" style="height: 36px">Filters</button>
        <Select bind:value={sort} options={SORTS} label="Sort results" style="width: 150px; height: 36px" />
      </div>
      <div class="list">
        {#if catalog.data}
          {#each results as r (r.id)}
            <div class="row res">
              <Tile art={r.art} size="lg" />
              <a href={href({ name: 'project', id: r.id })} style="min-width: 0; text-decoration: none"><span style="display: flex; gap: 8px; align-items: baseline; min-width: 0"><span class="trunc" style="font-weight: 600">{r.name}</span><span class="small faint trunc" style="flex: 0 1 auto">by {r.author}</span></span><span class="muted clamp2" style="display: -webkit-box; margin-top: 2px; height: 40px">{r.description}</span></a>
              <span class="opt" style="display: flex; flex-direction: column; gap: 2px"><span class="tnum muted small">{compact(r.downloads)} downloads</span><span class="small faint">Updated {ago(r.updated)}</span></span>
              <span style="display: flex; justify-content: flex-end"><button class="btn btn-sm" onclick={() => session.open({ kind: 'install', projectId: r.id })}>Install</button></span>
            </div>
          {:else}
            <Empty text={catalog.data.hits.length ? 'No projects match these filters.' : 'Browsing Modrinth and CurseForge arrives in a later update.'} />
          {/each}
        {:else}
          {#each SKELETON as [a, b], k (k)}
            <div class="row res" aria-hidden="true"><div class="sk" style="width: 48px; height: 48px; border-radius: 9px"></div><div style="display: flex; flex-direction: column; gap: 8px"><div class="sk" style="height: 12px; width: {a}"></div><div class="sk" style="height: 10px; width: 90%"></div><div class="sk" style="height: 10px; width: {b}"></div></div><div class="sk opt" style="height: 10px; width: 80%"></div><div class="sk" style="height: 28px; width: 64px; justify-self: end"></div></div>
          {/each}
        {/if}
      </div>
    </div>
  </section>
  <aside class="filters scroll" aria-label="Filters" style="border-left: 1px solid var(--line); padding: 16px; display: flex; flex-direction: column; gap: 18px">
    <div class="fg"><span class="field-label">Version and loader</span><div style="display: flex; gap: 6px"><span class="chip on">{target.data?.minecraft ?? ''}</span>{#if showTarget}<span class="chip on">Fabric</span>{/if}</div></div>
    <div class="fg"><span class="field-label">Categories</span>
      {#each catalog.data?.categories ?? [] as c, i (c)}
        <button class="fl" onclick={() => (cats[i] = !cats[i])} aria-pressed={!!cats[i]}><Check on={!!cats[i]} /><span>{c}</span><span></span></button>
      {/each}
    </div>
    <div class="fg"><span class="field-label">Hide projects with</span>
      <button class="fl" aria-pressed={hide.ai} onclick={() => (hide.ai = !hide.ai)}><Check on={hide.ai} /><span>AI-generated content</span><span></span></button>
      <button class="fl" aria-pressed={hide.ads} onclick={() => (hide.ads = !hide.ads)}><Check on={hide.ads} /><span>Ads or telemetry</span><span></span></button>
      <button class="fl" aria-pressed={hide.installed} onclick={() => (hide.installed = !hide.installed)}><Check on={hide.installed} /><span>Already installed</span><span></span></button>
    </div>
  </aside>
</div>

<style>
  /* Content-type tabs, search and results on the left; the filter sidebar sits to their right. */
  .bw{display:grid;grid-template-columns:minmax(0,1fr) 220px;flex:1 1 auto;min-height:0}
  @media (max-width:1100px){.bw{grid-template-columns:minmax(0,1fr)}.bw .filters{display:none}.fbtn{display:inline-flex !important}}
  .fbtn{display:none !important}
  .fg{display:flex;flex-direction:column;gap:6px}
  .fl{display:grid;grid-template-columns:16px minmax(0,1fr) auto;gap:8px;align-items:center;height:26px;border:0;background:transparent;padding:0;color:var(--muted);text-align:left;width:100%}
  .fl:hover{color:var(--text)}
  .res{grid-template-columns:48px minmax(0,1fr) 136px 84px;height:84px;column-gap:16px}
  @media (max-width:1100px){.res{grid-template-columns:48px minmax(0,1fr) 84px}.res .opt{display:none}}
</style>

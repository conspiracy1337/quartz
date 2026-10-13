<script lang="ts">
  import { untrack } from 'svelte';
  import { api } from '$lib/api';
  import { mergePage } from '$lib/browse';
  import { hasBridge } from '$lib/bridge';
  import type { BrowseSort, ProjectHit, ProjectKind, Src } from '$lib/api/types';
  import { ago, compact, errorText } from '$lib/format';
  import { LOADER_NAMES, PROJECT_TYPES, projectType } from '$lib/labels';
  import { artOf } from '$lib/look';
  import { query } from '$lib/query.svelte';
  import { go, href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Check from '$lib/ui/Check.svelte';
  import PageHeader from '$lib/ui/PageHeader.svelte';
  import SearchField from '$lib/ui/SearchField.svelte';
  import Select from '$lib/ui/Select.svelte';
  import Tabs from '$lib/ui/Tabs.svelte';
  import Tile from '$lib/ui/Tile.svelte';
  import Empty from '$lib/ui/Empty.svelte';

  /** Opened on a type and an instance from a content tab's "Add" button. */
  interface Props { type?: ProjectKind | undefined; instance?: string | undefined }
  let { type: startType, instance: startInstance }: Props = $props();

  const SORTS = [{ value: 'relevance', label: 'Relevance' }, { value: 'downloads', label: 'Downloads' }, { value: 'updated', label: 'Recently updated' }] as const;
  const SKELETON: [string, string][] = [['30%', '60%'], ['24%', '50%'], ['36%', '70%'], ['28%', '40%'], ['32%', '64%'], ['40%', '56%'], ['26%', '48%']];

  // svelte-ignore state_referenced_locally
  let type = $state<ProjectKind>(startType ?? 'mod');
  let search = $state('');
  let text = $state('');
  let sort = $state<BrowseSort>('relevance');
  // The design's board shows its first category ticked; the preview keeps that demo state.
  let cats = $state<Record<string, boolean>>(hasBridge() ? {} : { Optimization: true });
  let useVersion = $state(true);
  let useLoader = $state(true);
  const hide = session.browseHide;

  // svelte-ignore state_referenced_locally
  if (startInstance) session.browseTarget = startInstance;
  const instances = query(() => api.instances.list());
  const target = $derived(instances.data?.find((i) => i.id === session.browseTarget) ?? instances.data?.[0]);
  const showTarget = $derived(type !== 'modpack');
  const categories = query(() => api.browse.categories(type));

  // Typing searches once it pauses.
  $effect(() => {
    const v = search;
    const t = setTimeout(() => (text = v), 300);
    return () => clearTimeout(t);
  });

  let rows = $state<ProjectHit[] | undefined>(undefined);
  let searchId = '';
  let more = $state(false);
  let loadingMore = $state(false);
  let failed = $state('');
  let seq = 0;

  $effect(() => {
    const q = {
      type, text, sort, instance: target?.id ?? '', version: useVersion && !!target, loader: useLoader && showTarget && !!target?.loader,
      categories: Object.keys(cats).filter((c) => cats[c]), hideAi: hide.ai, hideAds: hide.ads, hideInstalled: hide.installed && showTarget,
    };
    untrack(() => void run(q));
  });

  async function run(q: Parameters<typeof api.browse.search>[0]) {
    const n = ++seq;
    rows = undefined;
    failed = '';
    try {
      const r = await api.browse.search(q);
      if (n !== seq) return;
      rows = r.rows;
      searchId = r.searchId;
      more = r.more;
    } catch (e) {
      if (n !== seq) return;
      rows = [];
      more = false;
      failed = errorText(e);
    }
  }

  /** Appends the next page; rows a twin merged into change in place, so nothing moves. */
  async function loadMore() {
    if (!more || loadingMore || !rows) return;
    const n = seq;
    loadingMore = true;
    try {
      const r = await api.browse.more(searchId);
      if (n !== seq || !rows) return;
      rows = mergePage(rows, r);
      more = r.more;
    } catch {
      more = false;
    } finally {
      loadingMore = false;
    }
  }

  function sentinel(node: HTMLElement) {
    const io = new IntersectionObserver((e) => e.some((x) => x.isIntersecting) && void loadMore(), { rootMargin: '400px' });
    io.observe(node);
    return { destroy: () => io.disconnect() };
  }

  // Results merge both sources: downloads are added up, and the date is the most recent update on either side.
  const merged = (p: ProjectHit) => {
    const srcs = [p.modrinth, p.curseforge].filter((x) => x !== undefined);
    return { downloads: srcs.reduce((n, x) => n + x.downloads, 0), updated: Math.max(...srcs.map((x) => x.updated)) };
  };
  /** The platform a row installs from: the one updated more recently, as the project page defaults to. */
  const fresher = (p: ProjectHit): Src => ((p.curseforge?.updated ?? 0) > (p.modrinth?.updated ?? 0) ? 'curseforge' : 'modrinth');

  function pickType(t: ProjectKind) {
    type = t;
    cats = {};
  }

  function install(r: ProjectHit) {
    const t = projectType(type);
    if (!t.content) return go({ name: 'import', ref: r.ref, source: fresher(r) });
    if (target) session.open({ kind: 'install', ref: r.ref, source: fresher(r), instanceId: target.id, content: t.content });
  }
</script>

<PageHeader title="Browse" style="padding-bottom: 16px; border-bottom: 1px solid var(--line)">
  {#snippet actions()}
    {#if showTarget && target}
      {@const t = target}
      <span class="small faint">Adding to</span>
      <Select value={t.id} options={(instances.data ?? []).map((i) => ({ value: i.id, label: i.name }))} label="Instance to add to" style="width: 200px" onchange={(v) => (session.browseTarget = v)}>
        {#snippet lead()}<Tile art={t.art} icon={t.icon} size="sm" />{/snippet}
      </Select>
    {/if}
  {/snippet}
</PageHeader>
<div class="bw">
  <section class="scroll" aria-label="Results">
    <div class="page" style="gap: 12px; padding-top: 4px">
      <Tabs items={PROJECT_TYPES.map((t) => ({ key: t.kind, label: t.label }))} active={type} label="Content type" onselect={pickType} />
      <div style="display: flex; gap: 8px">
        <SearchField bind:value={search} label="Search" placeholder="Search {projectType(type).label.toLowerCase()}" style="flex: 1 1 auto" height={36} />
        <button class="btn fbtn" style="height: 36px">Filters</button>
        <Select bind:value={sort} options={SORTS} label="Sort results" style="width: 150px; height: 36px" />
      </div>
      <div class="list">
        {#if rows}
          {#each rows as r (r.id)}
            {@const m = merged(r)}
            <div class="row res">
              <Tile art={r.art ?? artOf(r.name, r.icon)} size="lg" />
              <a href={href({ name: 'project', id: r.ref })} style="min-width: 0; text-decoration: none"><span style="display: flex; gap: 8px; align-items: baseline; min-width: 0"><span class="trunc" style="font-weight: 600">{r.name}</span><span class="small faint trunc" style="flex: 0 1 auto">by {r.author}</span></span><span class="muted clamp2" style="display: -webkit-box; margin-top: 2px; height: 40px">{r.summary}</span></a>
              <span class="opt" style="display: flex; flex-direction: column; gap: 2px"><span class="tnum muted small">{compact(m.downloads)} downloads</span><span class="small faint">Updated {ago(m.updated)}</span></span>
              <span style="display: flex; justify-content: flex-end"><button class="btn btn-sm" disabled={showTarget && !target} onclick={() => install(r)}>Install</button></span>
            </div>
          {:else}
            <Empty text={failed || 'No projects match these filters.'} />
          {/each}
          {#if more}<div use:sentinel aria-hidden="true"></div>{/if}
        {/if}
        {#if !rows || loadingMore}
          {#each loadingMore ? SKELETON.slice(0, 3) : SKELETON as [a, b], k (k)}
            <div class="row res" aria-hidden="true"><div class="sk" style="width: 48px; height: 48px; border-radius: 9px"></div><div style="display: flex; flex-direction: column; gap: 8px"><div class="sk" style="height: 12px; width: {a}"></div><div class="sk" style="height: 10px; width: 90%"></div><div class="sk" style="height: 10px; width: {b}"></div></div><div class="sk opt" style="height: 10px; width: 80%"></div><div class="sk" style="height: 28px; width: 64px; justify-self: end"></div></div>
          {/each}
        {/if}
      </div>
    </div>
  </section>
  <aside class="filters scroll" aria-label="Filters" style="border-left: 1px solid var(--line); padding: 16px; display: flex; flex-direction: column; gap: 18px">
    <div class="fg"><span class="field-label">Version and loader</span><div style="display: flex; gap: 6px">{#if target}<button class="chip" class:on={useVersion} aria-pressed={useVersion} onclick={() => (useVersion = !useVersion)}>{target.minecraft}</button>{#if showTarget && target.loader}<button class="chip" class:on={useLoader} aria-pressed={useLoader} onclick={() => (useLoader = !useLoader)}>{LOADER_NAMES[target.loader.kind]}</button>{/if}{/if}</div></div>
    <div class="fg"><span class="field-label">Categories</span>
      {#each categories.data ?? [] as c (c)}
        <button class="fl" onclick={() => (cats[c] = !cats[c])} aria-pressed={!!cats[c]}><Check on={!!cats[c]} /><span>{c}</span><span></span></button>
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

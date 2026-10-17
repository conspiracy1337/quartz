<script lang="ts">
  import { untrack } from 'svelte';
  import { api } from '$lib/api';
  import { mergePage } from '$lib/browse';
  import { hasBridge } from '$lib/bridge';
  import type { BrowseSort, ProjectHit, ProjectKind, Src } from '$lib/api/types';
  import { ago, compact, errorText } from '$lib/format';
  import { LOADER_NAMES, PROJECT_TYPES, SRC_NAMES, projectType } from '$lib/labels';
  import { artOf } from '$lib/look';
  import { query, rememberCount, skeletonRows, warm } from '$lib/query.svelte';
  import { PROJECT_KINDS, go, href, isPackSource } from '$lib/router.svelte';
  import { installInto } from '$lib/instanceActions';
  import { browseTarget } from '$lib/instances';
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
  // Below 1100px the filter sidebar opens over the results; Escape or a click outside closes it.
  let filtersOpen = $state(false);
  let filtersEl = $state<HTMLElement>();
  function closeFilters(e: Event) {
    if (!filtersOpen) return;
    if (e instanceof KeyboardEvent ? e.key === 'Escape' : !filtersEl?.contains(e.target as Node) && !(e.target as Element).closest?.('.fbtn')) filtersOpen = false;
  }
  // The design's board shows its first category ticked; the preview keeps that demo state.
  let cats = $state<Record<string, boolean>>(hasBridge() ? {} : { Optimization: true });
  let useVersion = $state(true);
  let useLoader = $state(true);
  /** Runs on: only client or only server projects ("" for any). */
  let side = $state<'' | 'client' | 'server'>('');
  /** Modpacks become new instances, so their version is picked by hand ("" for any). */
  let game = $state('');
  const releases = query(() => api.versions.minecraft(), 'versions.minecraft');
  const GAMES = $derived([{ value: '', label: 'Any version' }, ...(releases.data ?? []).flatMap((f) => f.releases.map((v) => ({ value: v.id, label: v.id })))]);
  const hide = session.browseHide;

  // svelte-ignore state_referenced_locally
  if (startInstance) session.addTo(startInstance);
  const instances = query(() => api.instances.list(), 'instances.list');
  const target = $derived(browseTarget(instances.data ?? [], session.browseTarget));
  const showTarget = $derived(type !== 'modpack');
  // Worlds come from CurseForge alone, untracked: no loader, side or Modrinth-only filters apply.
  const isWorld = $derived(type === 'world');
  // Browse is remounted for each type, so the key follows it.
  // svelte-ignore state_referenced_locally
  const categories = query(() => api.browse.categories(type), `browse.categories.${type}`);
  // The other tabs' categories are fixed lists: load them now, so switching tabs shows them at once.
  for (const k of PROJECT_KINDS) warm(`browse.categories.${k}`, () => api.browse.categories(k));
  // Each group's size too, so the filter skeleton has the loaded height next time.
  $effect(() => categories.data?.forEach((g, k) => rememberCount(`browse.categories.${type}.${k}`, g.names.length)));
  // Results the search marked as not installed flip once their download lands.
  const owned = query(() => (showTarget && target ? api.browse.installed(target.id) : Promise.resolve([])));
  const isInstalled = (r: ProjectHit) => r.installed || r.ref.split('+').some((p) => owned.data?.includes(p));

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
  /** Platforms that couldn't be searched; the rows shown come from the others. */
  let lost = $state<Src[]>([]);
  let seq = 0;

  $effect(() => {
    const q = {
      type, text, sort, instance: showTarget ? (target?.id ?? '') : '', version: useVersion && showTarget && !!target, game: showTarget ? '' : game,
      loader: useLoader && showTarget && !isWorld && !!target?.loader,
      categories: Object.keys(cats).filter((c) => cats[c]), hideAi: hide.ai && !isWorld, hideAds: hide.ads && !isWorld, hideInstalled: hide.installed && showTarget && !isWorld, side: isWorld ? '' : side,
    };
    untrack(() => void run(q));
  });

  async function run(q: Parameters<typeof api.browse.search>[0]) {
    const n = ++seq;
    rows = undefined;
    failed = '';
    lost = [];
    try {
      const r = await api.browse.search(q);
      if (n !== seq) return;
      rows = r.rows;
      lost = r.failed;
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
      lost = [...new Set([...lost, ...r.failed])];
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
    const srcs = [p.modrinth, p.curseforge, p.norisk].filter((x) => x !== undefined);
    return { downloads: srcs.reduce((n, x) => n + x.downloads, 0), updated: Math.max(...srcs.map((x) => x.updated)) };
  };

  function pickType(t: ProjectKind) {
    type = t;
    cats = {};
  }

  /** A row installs from the platform its project page defaults to. */
  async function install(r: ProjectHit) {
    const t = projectType(type);
    const into = t.content ? target : undefined;
    let source: Src = r.modrinth ? 'modrinth' : r.curseforge ? 'curseforge' : 'norisk';
    try {
      if (r.modrinth && r.curseforge) source = await api.browse.prefer(r.ref, into?.id ?? '');
    } catch (e) {
      return session.fail('Install', e);
    }
    if (!t.content && isPackSource(source)) {
      session.packHint = { ref: r.ref, name: r.name, author: r.author, summary: r.summary, ...(r.icon ? { icon: r.icon } : {}) };
      return go({ name: 'import', ref: r.ref, source });
    }
    if (into && t.content) void installInto(into.id, r.ref, source, t.content);
  }
</script>

<svelte:window onkeydown={closeFilters} onpointerdown={closeFilters} />

<PageHeader title="Browse" style="padding-bottom: 16px; border-bottom: 1px solid var(--line)" />
<div class="bw">
  <section class="scroll" aria-label="Results">
    <div class="page" style="gap: 12px; padding-top: 4px">
      <Tabs items={PROJECT_TYPES.map((t) => ({ key: t.kind, label: t.label }))} active={type} label="Content type" onselect={pickType} />
      <div style="display: flex; gap: 8px">
        <SearchField bind:value={search} label="Search" placeholder="Search {projectType(type).label.toLowerCase()}" style="flex: 1 1 auto" height={36} />
        <button class="btn fbtn" style="height: 36px" aria-expanded={filtersOpen} aria-controls="browse-filters" onclick={() => (filtersOpen = !filtersOpen)}>Filters</button>
        <Select bind:value={sort} options={SORTS} label="Sort results" style="width: 150px; height: 36px" />
      </div>
      {#if lost.length && rows?.length}<p class="small faint" style="margin: 0">{lost.map((x) => SRC_NAMES[x]).join(' and ')} couldn't be reached; showing {SRC_NAMES[lost[0] === 'modrinth' ? 'curseforge' : 'modrinth']} only.</p>{/if}
      <div class="list">
        {#if rows}
          {#each rows as r (r.id)}
            {@const m = merged(r)}
            <div class="row res">
              <Tile art={r.art ?? artOf(r.name, r.icon)} size="lg" />
              <a href={href({ name: 'project', id: r.ref })} style="min-width: 0; text-decoration: none"><span style="display: flex; gap: 8px; align-items: baseline; min-width: 0"><span class="trunc" style="font-weight: 600">{r.name}</span><span class="small faint trunc" style="flex: 0 1 auto">by {r.author}</span></span><span class="muted clamp2" style="display: -webkit-box; margin-top: 2px; height: 40px">{r.summary}</span></a>
              <span class="opt" style="flex-direction: column; gap: 2px"><span class="tnum muted small">{r.norisk ? `From ${SRC_NAMES.norisk}` : `${compact(m.downloads)} downloads`}</span><span class="small faint">Updated {ago(m.updated)}</span></span>
              <span style="display: flex; justify-content: flex-end">{#if isInstalled(r)}<span class="small" style="color: var(--accent)">Installed</span>{:else}<button class="btn btn-sm" disabled={showTarget && !target} onclick={() => install(r)}>Install</button>{/if}</span>
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
  <aside id="browse-filters" class="filters scroll" class:open={filtersOpen} aria-label="Filters" bind:this={filtersEl} style="border-left: 1px solid var(--line); padding: 16px; flex-direction: column; gap: 18px">
    {#if showTarget && !instances.data}
      <div class="fg" aria-hidden="true"><span class="field-label">Adding to</span><div class="sk" style="height: 32px; border-radius: 6px"></div></div>
    {:else if showTarget && target}
      {@const t = target}
      <div class="fg"><span class="field-label">Adding to</span>
        <Select value={t.id} options={(instances.data ?? []).map((i) => ({ value: i.id, label: i.name }))} label="Instance to add to" style="width: 100%" onchange={(v) => session.addTo(v)}>
          {#snippet lead()}<Tile art={t.art} icon={t.icon} size="sm" />{/snippet}
        </Select>
      </div>
    {/if}
    {#if showTarget}
      <div class="fg"><span class="field-label">{isWorld ? 'Minecraft version' : 'Version and loader'}</span><div style="display: flex; gap: 6px">{#if target}<button class="chip" class:on={useVersion} aria-pressed={useVersion} onclick={() => (useVersion = !useVersion)}>{target.minecraft}</button>{#if target.loader && !isWorld}<button class="chip" class:on={useLoader} aria-pressed={useLoader} onclick={() => (useLoader = !useLoader)}>{LOADER_NAMES[target.loader.kind]}</button>{/if}{:else if !instances.data}<span class="sk" style="width: 56px; height: 24px; border-radius: 12px" aria-hidden="true"></span>{/if}</div></div>
    {:else}
      <div class="fg"><span class="field-label">Minecraft version</span><Select bind:value={game} options={GAMES} label="Minecraft version" style="width: 100%" /></div>
    {/if}
    {#if !categories.data}
      {#each { length: skeletonRows(`browse.categories.${type}`, 1) } as _, k (k)}
        <div class="fg" aria-hidden="true"><div class="sk" style="height: 12px; width: 72px; margin: 4px 0"></div>{#each { length: skeletonRows(`browse.categories.${type}.${k}`, 6) } as _, j (j)}<div class="fl"><div class="sk" style="width: 16px; height: 16px; border-radius: 4px"></div><div class="sk" style="height: 10px; width: {[60, 48, 70, 54, 64, 44][j % 6]}%"></div><span></span></div>{/each}</div>
      {/each}
    {/if}
    {#each categories.data ?? [] as g (g.title)}
      <div class="fg"><span class="field-label">{g.title}</span>
        {#each g.names as c (c)}
          <button class="fl" onclick={() => (cats[c] = !cats[c])} aria-pressed={!!cats[c]}><Check on={!!cats[c]} /><span>{c}</span><span></span></button>
        {/each}
      </div>
    {/each}
    {#if !isWorld}
    <div class="fg"><span class="field-label">Runs on</span>
      {#each [['client', 'Client side'], ['server', 'Server side']] as const as [v, l] (v)}
        <button class="fl" aria-pressed={side === v} onclick={() => (side = side === v ? '' : v)}><Check on={side === v} /><span>{l}</span><span></span></button>
      {/each}
    </div>
    <div class="fg"><span class="field-label">Hide projects with</span>
      <button class="fl" aria-pressed={hide.ai} onclick={() => (hide.ai = !hide.ai)}><Check on={hide.ai} /><span>AI-generated content</span><span></span></button>
      <button class="fl" aria-pressed={hide.ads} onclick={() => (hide.ads = !hide.ads)}><Check on={hide.ads} /><span>Ads or telemetry</span><span></span></button>
      {#if showTarget}<button class="fl" aria-pressed={hide.installed} onclick={() => (hide.installed = !hide.installed)}><Check on={hide.installed} /><span>Already installed</span><span></span></button>{/if}
    </div>
    {/if}
  </aside>
</div>

<style>
  /* Content-type tabs, search and results on the left; the filter sidebar sits to their right. */
  .bw{display:grid;grid-template-columns:minmax(0,1fr) 220px;flex:1 1 auto;min-height:0;position:relative}
  .bw .filters{display:flex}
  .fbtn{display:none !important}
  /* Narrow windows: the sidebar hides behind the Filters button and opens over the results, on the menu surface. */
  @media (max-width:1100px){.bw{grid-template-columns:minmax(0,1fr)}.bw .filters{display:none}.fbtn{display:inline-flex !important}
    .bw .filters.open{display:flex;position:absolute;top:0;right:0;bottom:0;width:220px;z-index:20;background:var(--raise);border-left-color:var(--line-2) !important;box-shadow:0 16px 40px -12px rgba(0,0,0,.7)}}
  .fg{display:flex;flex-direction:column;gap:6px}
  .fl{display:grid;grid-template-columns:16px minmax(0,1fr) auto;gap:8px;align-items:center;height:26px;border:0;background:transparent;padding:0;color:var(--muted);text-align:left;width:100%}
  .fl:hover{color:var(--text)}
  .res{grid-template-columns:48px minmax(0,1fr) 136px 84px;height:84px;column-gap:16px}
  .res .opt{display:flex}
  @media (max-width:1100px){.res{grid-template-columns:48px minmax(0,1fr) 84px}.res .opt{display:none}}
</style>

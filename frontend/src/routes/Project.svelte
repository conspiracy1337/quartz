<script lang="ts">
  import { api } from '$lib/api';
  import type { ProjectVersion, Src } from '$lib/api/types';
  import { ago, bytes, compact, errorText, shortMonthDay, shortDate } from '$lib/format';
  import { LOADER_NAMES, SRC_NAMES, projectType } from '$lib/labels';
  import { artOf } from '$lib/look';
  import { query } from '$lib/query.svelte';
  import { go, href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Inline from '$lib/ui/Inline.svelte';
  import type { IconName } from '$lib/ui/icons';
  import Menu from '$lib/ui/Menu.svelte';
  import Tabs from '$lib/ui/Tabs.svelte';
  import Tile from '$lib/ui/Tile.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';

  /** A project page; `id` is its reference on one or both platforms. */
  interface Props { id: string; tab?: 'versions' | undefined }
  let { id, tab: startTab }: Props = $props();

  const CHANNELS = ['Release', 'Beta', 'Alpha'] as const;
  const SOURCES = ['modrinth', 'curseforge'] as const;

  const instances = query(() => api.instances.list());
  const target = $derived(instances.data?.find((i) => i.id === session.browseTarget) ?? instances.data?.[0]);
  const project = query(() => api.browse.project(id, target?.id ?? ''));
  // svelte-ignore state_referenced_locally
  let tab = $state(startTab === 'versions' ? 1 : 0);
  let openVersion = $state(0);
  let onlyCompatible = $state(true);
  let channels = $state({ Release: true, Beta: true, Alpha: false });
  let chosen = $state<Src | null>(null);
  let chooser = $state<{ x: number; y: number } | null>(null);

  const p = $derived(project.data);
  const src = $derived<Src>(chosen ?? p?.fresher ?? 'modrinth');
  const other = $derived<Src>(src === 'modrinth' ? 'curseforge' : 'modrinth');
  const side = $derived(p?.[src] ?? p?.[other]);
  const both = $derived(!!p?.modrinth && !!p?.curseforge);
  const why = $derived(
    p?.modrinth && p.curseforge ? (src === p.fresher
      ? `more recently (${ago(p[src]!.updated)}, ${SRC_NAMES[other]} ${ago(p[other]!.updated)})`
      : `${ago(p[src]!.updated)} (${SRC_NAMES[other]} was updated more recently)`) : '',
  );
  const versions = query(() => (p?.[src] ? api.browse.versions(id, src, target?.id ?? '', onlyCompatible) : Promise.resolve([] as ProjectVersion[])));
  const shown = $derived((versions.data ?? []).filter((v) => channels[v.channel]));
  const changes = query(() => (tab === 3 && p?.[src] ? api.browse.changelog(id, src, []) : Promise.resolve([])));
  const openId = $derived(shown[openVersion]?.id);
  const notes = query(() => (tab === 1 && openId ? api.browse.changelog(id, src, [openId]) : Promise.resolve([])));
  const tabs = $derived([
    { key: 0, label: 'Description' },
    { key: 1, label: 'Versions', count: side?.versions || versions.data?.length },
    { key: 2, label: 'Gallery', count: side?.gallery.length },
    { key: 3, label: 'Changelog' },
  ]);
  /** The newest version for the target that the installed one would update to. */
  const latest = $derived(versions.data?.find((v) => v.action === 'Update'));
  const kind = $derived(p ? projectType(p.type) : undefined);
  const forTarget = $derived(target ? `${target.name} (${target.minecraft}${target.loader ? `, ${LOADER_NAMES[target.loader.kind]}` : ''})` : '');

  function install(version?: string) {
    if (!p || !kind) return;
    if (!kind.content) return go({ name: 'import', ref: p.ref, source: src });
    if (target) session.open({ kind: 'install', ref: p.ref, source: src, instanceId: target.id, content: kind.content, ...(version ? { version } : {}) });
  }
  function choose(e: MouseEvent) {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    chooser = { x: r.right - 240, y: r.bottom + 4 };
  }
  const open = (url: string) => (e: MouseEvent) => {
    e.preventDefault();
    void api.app.openUrl(url);
  };
</script>

<div class="scroll">
  <div class="page" style="gap: 18px">
    {#if p && side}
      <div class="crumbs"><a href={href({ name: 'browse', type: p.type })}>Browse</a><Icon name="chevron-right" size={12} sw={2} /><span class="muted">{p.name}</span></div>
      <header style="display: grid; grid-template-columns: 72px minmax(0,1fr) auto auto; gap: 18px; align-items: center">
        <Tile art={side.art ?? artOf(side.name, side.icon)} style="width: 72px; height: 72px; border-radius: 14px; font-size: 28px" />
        <div style="min-width: 0; display: flex; flex-direction: column; gap: 6px">
          <h1 class="h1 trunc">{side.name}</h1>
          <div class="muted trunc">{side.summary}</div>
          <div style="display: flex; gap: 6px; flex-wrap: wrap">{#each side.badges as b (b)}<span class="badge">{b}</span>{/each}<span class="small faint tnum" style="margin-left: 6px">by {side.author}, {compact(side.downloads)} downloads on {SRC_NAMES[p[src] ? src : other]}</span></div>
        </div>
        <div class="seg" role="group" aria-label="Source" style="margin-right: 4px">
          {#each SOURCES as s (s)}<button class:on={src === s} onclick={() => (chosen = s)} aria-pressed={src === s} disabled={!p[s]}>{SRC_NAMES[s]}</button>{/each}
        </div>
        <div style="display: flex; gap: 0">
          <button class="btn btn-accent btn-lg" style="border-radius: 6px 0 0 6px" disabled={!!kind?.content && !target} onclick={() => install()}>Install</button>
          <button class="btn btn-accent btn-lg btn-icon" style="border-radius: 0 6px 6px 0; border-left: 1px solid rgba(0,0,0,.2); width: 36px" aria-label="Choose instance" aria-haspopup="menu" disabled={!kind?.content} onclick={choose}><Icon name="chevron-down" size={12} sw={2.2} /></button>
        </div>
      </header>
      <Tabs items={tabs} active={tab} label="Project" onselect={(k) => (tab = k)} />

      {#if tab === 0}
        <div class="pa">
          <article class="md">
            {#if side.gallery[0]}{@const g = side.gallery[0]}<div class="thumb" style="height: 200px; margin-bottom: 18px; background: {g.art ?? `url(${g.src}) center/cover no-repeat, var(--raise)`}" role="img" aria-label={g.title}></div>{/if}
            {#each side.body as b, bi (bi)}
              {#if b.type === 'ul'}<ul>{#each b.items as it, ii (ii)}<li><Inline spans={it} /></li>{/each}</ul>
              {:else if b.type === 'h2'}<h2><Inline spans={b.spans} /></h2>
              {:else if b.type === 'img'}<div class="thumb" style="height: 200px; margin-bottom: 12px; background: url({b.src}) center/contain no-repeat, var(--raise)" role="img" aria-label={b.alt}></div>
              {:else}<p><Inline spans={b.spans} /></p>{/if}
            {/each}
          </article>
          <aside style="display: flex; flex-direction: column; gap: 18px">
            {#if target && p.installed}
              <section style="display: flex; flex-direction: column; gap: 6px"><h2 class="h3">For {target.name}</h2>
                <div class="small muted">{`Version ${p.installed.version.split('+')[0]} is installed.${latest ? ` ${latest.number.split('+')[0]} is available.` : ''}`}</div>
                {#if latest}<a href={href({ name: 'instance', id: target.id, tab: kind?.content ?? 'mods', sub: 'updates' })} class="btn btn-sm">Update to {latest.number.split('+')[0]}</a>{/if}
              </section>
              <div class="divider"></div>
            {/if}
            <nav aria-label="Links" style="display: flex; flex-direction: column">
              {#each side.links as l (l.url)}
                <a class="lk" href={l.url} onclick={open(l.url)}><Icon name={l.icon as IconName} size={14} />{l.label}</a>
              {/each}
              <a class="lk" href={side.url} onclick={open(side.url)}><Icon name="external" size={14} />View on {SRC_NAMES[p[src] ? src : other]}</a>
              {#if both}<span class="small faint" style="margin-top: 6px">Showing {SRC_NAMES[src]} because it was updated {why}.</span>{/if}
            </nav>
          </aside>
        </div>
      {:else if tab === 1}
        <div style="display: flex; flex-wrap: wrap; gap: 8px; align-items: center">
          <label class="small muted" style="display: flex; gap: 8px; align-items: center"><Toggle bind:on={onlyCompatible} label="Only compatible versions" onchange={() => (openVersion = -1)} />Only versions for {forTarget}</label>
          <div style="flex: 1 1 0"></div>
          {#each CHANNELS as c (c)}<button class="chip" class:on={channels[c]} onclick={() => (channels[c] = !channels[c])}>{c}</button>{/each}
        </div>
        <div class="list">
          <div class="row row-head pv"><span>Version</span><span>Channel</span><span class="opt">Minecraft</span><span class="opt">Published</span><span class="opt">Size</span><span></span></div>
          {#each shown as v, i (v.id)}
            <div>
              <div class="row pv">
                <button class="pv-id mono trunc" onclick={() => (openVersion = openVersion === i ? -1 : i)} aria-expanded={openVersion === i}>{v.number}</button>
                <span><span class="badge" class:badge-accent={v.channel === 'Release'} class:badge-warn={v.channel !== 'Release'}>{v.channel}</span></span>
                <span class="mono muted opt trunc">{v.minecraft}</span>
                <span class="muted opt tnum">{shortMonthDay(v.published)}, {new Date(v.published).getFullYear()}</span>
                <span class="mono muted opt">{bytes(v.bytes)}</span>
                <span style="display: flex; justify-content: flex-end">
                  {#if !v.action}<span class="small" style="color: var(--accent)">Installed</span>{:else}<button class="btn btn-sm" disabled={!!kind?.content && !target} onclick={() => install(v.id)}>{v.action}</button>{/if}
                </span>
              </div>
              {#if openVersion === i}
                <div class="cl"><strong style="color: var(--text); font-weight: 500">Changes in {v.number}</strong><ul>{#each notes.data?.[0]?.items ?? [] as n, ni (ni)}<li>{n}</li>{/each}</ul></div>
              {/if}
            </div>
          {:else}
            {#if versions.data}<div class="row" style="height: 52px"><span class="small muted">{versions.error ? errorText(versions.error) : 'No versions match.'}</span></div>{/if}
          {/each}
          {#if !versions.data}
            {#each [0, 1, 2, 3] as k (k)}<div class="row pv" aria-hidden="true"><div class="sk" style="height: 12px; width: 60%"></div><div class="sk" style="height: 18px; width: 56px"></div><div class="sk opt" style="height: 10px; width: 50%"></div><div class="sk opt" style="height: 10px; width: 70%"></div><div class="sk opt" style="height: 10px; width: 60%"></div><div class="sk" style="height: 28px; width: 64px; justify-self: end"></div></div>{/each}
          {/if}
        </div>
      {:else if tab === 2}
        <div class="gal">
          {#each side.gallery as g, gi (gi)}<figure class="gal-item"><span class="gal-img" style="background: {g.art ?? `url(${g.src}) center/cover no-repeat, var(--raise)`}" role="img" aria-label={g.title}></span><figcaption class="small muted trunc">{g.title}</figcaption></figure>{/each}
        </div>
      {:else}
        <div class="md" style="display: flex; flex-direction: column; gap: 18px">
          {#each changes.data ?? [] as v (v.id)}
            <section><h2 style="margin-top: 0; display: flex; gap: 10px; align-items: baseline"><span class="mono">{v.number}</span><span class="small faint" style="font-weight: 400">{shortDate(v.published)}</span></h2><ul>{#each v.items as n, ni (ni)}<li>{n}</li>{/each}</ul></section>
          {/each}
        </div>
      {/if}
    {:else if project.error}
      <div class="crumbs"><a href={href({ name: 'browse' })}>Browse</a></div>
      <p class="muted" style="margin: 0">{errorText(project.error)}</p>
    {:else}
      <div class="sk" style="height: 20px; width: 160px" aria-hidden="true"></div>
      <div style="display: grid; grid-template-columns: 72px minmax(0,1fr); gap: 18px; align-items: center" aria-hidden="true">
        <div class="sk" style="width: 72px; height: 72px; border-radius: 14px"></div>
        <div style="display: flex; flex-direction: column; gap: 8px"><div class="sk" style="height: 20px; width: 30%"></div><div class="sk" style="height: 12px; width: 55%"></div><div class="sk" style="height: 12px; width: 40%"></div></div>
      </div>
    {/if}
  </div>
</div>

{#if chooser}
  <Menu x={chooser.x} y={chooser.y} label="Install into" title="Install into" onclose={() => (chooser = null)}>
    {#each instances.data ?? [] as i (i.id)}
      <button class="mi" role="menuitem" onclick={() => { session.browseTarget = i.id; chooser = null; }}>
        <Tile art={i.art} icon={i.icon} size="sm" /><span class="trunc">{i.name}</span><span class="small faint">{i.id === target?.id ? 'Selected' : ''}</span>
      </button>
    {/each}
  </Menu>
{/if}

<style>
  .pa{display:grid;grid-template-columns:minmax(0,1fr) 240px;gap:28px;align-items:start}
  @media (max-width:1100px){.pa{grid-template-columns:minmax(0,1fr)}}
  .md{max-width:68ch;color:var(--muted);font-size:14px;line-height:22px}
  .md h2{color:var(--text);font-size:16px;line-height:22px;margin:22px 0 8px;font-weight:600}
  .md p{margin:0 0 12px}
  .md ul{margin:0 0 12px;padding-left:18px}
  .md li{margin:4px 0}
  .md :global(code){font-family:var(--mono);font-size:12px;background:var(--raise);padding:1px 5px;border-radius:4px;color:var(--text)}
  .lk{display:flex;align-items:center;gap:10px;height:32px;color:var(--muted);text-decoration:none}
  .lk:hover{color:var(--text)}
  .pv{grid-template-columns:minmax(0,1fr) 72px 120px 104px 72px 96px;height:52px}
  @media (max-width:1100px){.pv{grid-template-columns:minmax(0,1fr) 72px 96px}.pv .opt{display:none}}
  .pv-id{all:unset;cursor:pointer;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;min-width:0}
  .pv-id:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  .cl{padding:12px 16px 16px;background:var(--bg);color:var(--muted);font-size:13px;line-height:20px}
  .cl ul{margin:6px 0 0;padding-left:18px}
  .gal{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:var(--gap)}
  @media (max-width:1100px){.gal{grid-template-columns:repeat(2,minmax(0,1fr))}}
  .gal-item{margin:0;display:flex;flex-direction:column;gap:8px;min-width:0}
  .gal-img{display:block;aspect-ratio:16/9;border-radius:var(--radius-lg);border:1px solid var(--line)}
</style>

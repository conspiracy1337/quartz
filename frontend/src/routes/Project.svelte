<script lang="ts">
  import { api } from '$lib/api';
  import { ago, bytes, compact, shortMonthDay, fullDate } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import { href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Inline from '$lib/ui/Inline.svelte';
  import type { IconName } from '$lib/ui/icons';
  import Tabs from '$lib/ui/Tabs.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';

  interface Props { id: string }
  let { id }: Props = $props();

  type Src = 'modrinth' | 'curseforge';
  const SRC_NAMES: Record<Src, string> = { modrinth: 'Modrinth', curseforge: 'CurseForge' };
  const CHANNELS = ['Release', 'Beta', 'Alpha'] as const;

  const project = query(() => api.browse.project(id));
  const target = query(() => api.instances.first());
  let tab = $state(0);
  let openVersion = $state(0);
  let onlyCompatible = $state(true);
  let channels = $state({ Release: true, Beta: true, Alpha: false });
  let chosen = $state<Src | null>(null);

  const p = $derived(project.data);
  // Defaults to whichever source was updated more recently.
  const fresher = $derived<Src>(p && p.sources.curseforge.updated > p.sources.modrinth.updated ? 'curseforge' : 'modrinth');
  const src = $derived(chosen ?? fresher);
  const other = $derived<Src>(src === 'modrinth' ? 'curseforge' : 'modrinth');
  const why = $derived(
    p ? (src === fresher
      ? `more recently (${ago(p.sources[src].updated)}, ${SRC_NAMES[other]} ${ago(p.sources[other].updated)})`
      : `${ago(p.sources[src].updated)} (${SRC_NAMES[other]} was updated more recently)`) : '',
  );
  const tabs = $derived([
    { key: 0, label: 'Description' },
    { key: 1, label: 'Versions', count: p?.versionCount },
    { key: 2, label: 'Gallery', count: p?.gallery.length },
    { key: 3, label: 'Changelog' },
  ]);
  const versions = $derived(
    (p?.versions ?? []).filter((v) => channels[v.channel] && (!onlyCompatible || v.minecraft.includes(target.data?.minecraft ?? ''))),
  );
  const install = () => session.open({ kind: 'install', projectId: id });
</script>

<div class="scroll">
  <div class="page" style="gap: 18px">
    {#if p}
      <div class="crumbs"><a href={href({ name: 'browse' })}>Browse</a><Icon name="chevron-right" size={12} sw={2} /><span class="muted">{p.name}</span></div>
      <header style="display: grid; grid-template-columns: 72px minmax(0,1fr) auto auto; gap: 18px; align-items: center">
        <span class="ico" style="width: 72px; height: 72px; border-radius: 14px; background: {p.art.background}; font-size: 28px">{p.art.initial}</span>
        <div style="min-width: 0; display: flex; flex-direction: column; gap: 6px">
          <h1 class="h1 trunc">{p.name}</h1>
          <div class="muted trunc">{p.tagline}</div>
          <div style="display: flex; gap: 6px; flex-wrap: wrap">{#each p.badges as b (b)}<span class="badge">{b}</span>{/each}<span class="small faint tnum" style="margin-left: 6px">by {p.author}, {compact(p.sources[src].downloads)} downloads on {SRC_NAMES[src]}</span></div>
        </div>
        <div class="seg" role="group" aria-label="Source" style="margin-right: 4px">
          {#each ['modrinth', 'curseforge'] as const as s (s)}<button class:on={src === s} onclick={() => (chosen = s)} aria-pressed={src === s}>{SRC_NAMES[s]}</button>{/each}
        </div>
        <div style="display: flex; gap: 0">
          <button class="btn btn-accent btn-lg" style="border-radius: 6px 0 0 6px" onclick={install}>Install</button>
          <button class="btn btn-accent btn-lg btn-icon" style="border-radius: 0 6px 6px 0; border-left: 1px solid rgba(0,0,0,.2); width: 36px" aria-label="Choose instance"><Icon name="chevron-down" size={12} sw={2.2} /></button>
        </div>
      </header>
      <Tabs items={tabs} active={tab} label="Project" onselect={(k) => (tab = k)} />

      {#if tab === 0}
        <div class="pa">
          <article class="md">
            <div class="thumb" style="height: 200px; margin-bottom: 18px; background: {p.gallery[0]?.art}" role="img" aria-label={p.gallery[0]?.title}></div>
            {#each p.body as b, bi (bi)}
              {#if b.type === 'ul'}<ul>{#each b.items as it (it)}<li><Inline text={it} /></li>{/each}</ul>
              {:else if b.type === 'h2'}<h2>{b.text}</h2>
              {:else}<p><Inline text={b.text} /></p>{/if}
            {/each}
          </article>
          <aside style="display: flex; flex-direction: column; gap: 18px">
            {#if target.data}
              <section style="display: flex; flex-direction: column; gap: 6px"><h2 class="h3">For {target.data.name}</h2><div class="small muted">Version {p.installed.split('+')[0]} is installed. {p.versions[0]?.id.split('+')[0]} is available.</div><a href={href({ name: 'instance', id: target.data.id, tab: 'mods', sub: 'updates' })} class="btn btn-sm">Update to {p.versions[0]?.id.split('+')[0]}</a></section>
            {/if}
            <div class="divider"></div>
            <nav aria-label="Links" style="display: flex; flex-direction: column">
              {#each p.links as l (l.url)}
                <a class="lk" href={l.url} onclick={(e) => { e.preventDefault(); void api.app.openUrl(l.url); }}><Icon name={l.icon as IconName} size={14} />{l.label}</a>
              {/each}
              <a class="lk" href={p.sources[src].url} onclick={(e) => { e.preventDefault(); void api.app.openUrl(p.sources[src].url); }}><Icon name="external" size={14} />View on {SRC_NAMES[src]}</a>
              <span class="small faint" style="margin-top: 6px">Showing {SRC_NAMES[src]} because it was updated {why}.</span>
            </nav>
          </aside>
        </div>
      {:else if tab === 1}
        <div style="display: flex; flex-wrap: wrap; gap: 8px; align-items: center">
          <label class="small muted" style="display: flex; gap: 8px; align-items: center"><Toggle bind:on={onlyCompatible} label="Only compatible versions" onchange={() => (openVersion = -1)} />Only versions for {target.data?.name} ({target.data?.minecraft}, Fabric)</label>
          <div style="flex: 1 1 0"></div>
          {#each CHANNELS as c (c)}<button class="chip" class:on={channels[c]} onclick={() => (channels[c] = !channels[c])}>{c}</button>{/each}
        </div>
        <div class="list">
          <div class="row row-head pv"><span>Version</span><span>Channel</span><span class="opt">Minecraft</span><span class="opt">Published</span><span class="opt">Size</span><span></span></div>
          {#each versions as v, i (v.id)}
            {@const installed = v.id === p.installed}
            <div>
              <div class="row pv">
                <button class="pv-id mono trunc" onclick={() => (openVersion = openVersion === i ? -1 : i)} aria-expanded={openVersion === i}>{v.id}</button>
                <span><span class="badge" class:badge-accent={v.channel === 'Release'} class:badge-warn={v.channel !== 'Release'}>{v.channel}</span></span>
                <span class="mono muted opt trunc">{v.minecraft}</span>
                <span class="muted opt tnum">{shortMonthDay(v.published)}, {new Date(v.published).getFullYear()}</span>
                <span class="mono muted opt">{bytes(v.bytes)}</span>
                <span style="display: flex; justify-content: flex-end">
                  {#if installed}<span class="small" style="color: var(--accent)">Installed</span>{:else}<button class="btn btn-sm" onclick={install}>{i < 2 ? 'Update' : 'Switch'}</button>{/if}
                </span>
              </div>
              {#if openVersion === i}
                <div class="cl"><strong style="color: var(--text); font-weight: 500">Changes in {v.id}</strong><ul>{#each v.notes as n (n)}<li>{n}</li>{/each}</ul></div>
              {/if}
            </div>
          {/each}
        </div>
      {:else if tab === 2}
        <div class="gal">
          {#each p.gallery as g (g.title)}<figure class="gal-item"><span class="gal-img" style="background: {g.art}" role="img" aria-label={g.title}></span><figcaption class="small muted trunc">{g.title}</figcaption></figure>{/each}
        </div>
      {:else}
        <div class="md" style="display: flex; flex-direction: column; gap: 18px">
          {#each p.versions.slice(0, 4) as v (v.id)}
            <section><h2 style="margin-top: 0; display: flex; gap: 10px; align-items: baseline"><span class="mono">{v.id}</span><span class="small faint" style="font-weight: 400">{fullDate(v.published)}</span></h2><ul>{#each v.notes as n (n)}<li>{n}</li>{/each}</ul></section>
          {/each}
        </div>
      {/if}
    {:else}
      <div class="sk" style="height: 20px; width: 160px" aria-hidden="true"></div>
      <div style="display: grid; grid-template-columns: 72px minmax(0,1fr); gap: 18px; align-items: center" aria-hidden="true">
        <div class="sk" style="width: 72px; height: 72px; border-radius: 14px"></div>
        <div style="display: flex; flex-direction: column; gap: 8px"><div class="sk" style="height: 20px; width: 30%"></div><div class="sk" style="height: 12px; width: 55%"></div><div class="sk" style="height: 12px; width: 40%"></div></div>
      </div>
    {/if}
  </div>
</div>

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

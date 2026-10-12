<script lang="ts">
  import { api } from '../api';
  import type { ContentItem, ContentKind, ContentStatus, Source } from '../api/types';
  import { searchIn } from '../format';
  import { query } from '../query.svelte';
  import { href } from '../router.svelte';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';
  import SearchField from '../ui/SearchField.svelte';
  import Toggle from '../ui/Toggle.svelte';
  import Empty from '../ui/Empty.svelte';

  /** The one list used by Mods, Resource packs, Shaders, Data packs, Worlds and Servers. */
  interface Props { instanceId: string; kind: ContentKind }
  let { instanceId, kind }: Props = $props();

  type Chip = { label: string; test: (c: ContentItem) => boolean };
  interface Config { noun: string; secondary: string; primary: string; toggles: boolean; exclusive?: boolean; hint?: string; projects: boolean; chips: (items: ContentItem[]) => Chip[] }
  const all: Chip = { label: 'All', test: () => true };
  const CONFIG: Record<ContentKind, Config> = {
    mods: { noun: 'mods', secondary: 'Check updates', primary: 'Add mods', toggles: true, projects: true,
      chips: () => [all, { label: 'Updates', test: (c) => c.status?.kind === 'update' }, { label: 'Disabled', test: (c) => !c.enabled }] },
    resourcepacks: { noun: 'resource packs', secondary: 'Check updates', primary: 'Add resource packs', toggles: true, projects: true,
      hint: 'Enabled packs higher in the list override the ones below. Drag a row to reorder.',
      chips: () => [all, { label: 'Enabled', test: (c) => c.enabled }, { label: 'Disabled', test: (c) => !c.enabled }] },
    shaders: { noun: 'shader packs', secondary: 'Check updates', primary: 'Add shaders', toggles: true, exclusive: true, projects: true,
      hint: 'Only one shader pack runs at a time. Uses Iris 1.8.1.', chips: () => [all] },
    datapacks: { noun: 'data packs', secondary: 'Check updates', primary: 'Add data packs', toggles: true, projects: true,
      chips: (items) => [{ label: 'All worlds', test: () => true }, ...[...new Set(items.map((c) => c.group).filter((g) => g !== undefined))].map((g) => ({ label: g, test: (c: ContentItem) => c.group === g }))] },
    worlds: { noun: 'worlds', secondary: 'Open folder', primary: 'Import world', toggles: false, projects: false,
      hint: 'Worlds are backed up before each launch. The last 5 backups are kept.',
      chips: () => [all, { label: 'Survival', test: (c) => c.group === 'Survival' }, { label: 'Creative', test: (c) => c.group === 'Creative' }] },
    servers: { noun: 'servers', secondary: 'Refresh', primary: 'Add server', toggles: false, projects: false,
      chips: () => [all, { label: 'Online', test: (c) => c.online === true }] },
  };
  const SOURCES: Record<Source, [string, string]> = { modrinth: ['M', 'Modrinth'], curseforge: ['C', 'CurseForge'], file: ['F', 'Added from file'] };
  const SKELETON: [string, string][] = [['42%', '22%'], ['30%', '24%'], ['48%', '18%'], ['36%', '20%'], ['56%', '30%'], ['40%', '16%'], ['44%', '26%'], ['32%', '20%']];
  const CHECK_MS = 1400;

  const cfg = $derived(CONFIG[kind]);
  const EMPTY: Record<ContentKind, string> = {
    mods: 'No mods yet.', resourcepacks: 'No resource packs yet.', shaders: 'No shader packs yet.',
    datapacks: 'No data packs yet.', worlds: 'No worlds yet. They show up here after you play.', servers: 'No servers yet.',
  };
  const items = query(() => api.content.list(instanceId, kind));
  let rows = $state<ContentItem[]>([]);
  $effect(() => {
    rows = items.data ?? [];
  });
  let chip = $state(0);
  let search = $state('');
  let checking = $state(false);
  // Shaders: exactly one pack is active at a time.
  let active = $state<string | null>(null);
  $effect(() => {
    active = cfg.exclusive ? (items.data?.find((c) => c.enabled)?.id ?? null) : null;
  });

  const chips = $derived(cfg.chips(rows));
  const shown = $derived(rows.filter((c) => (chips[chip] ?? all).test(c) && c.name.toLowerCase().includes(search.trim().toLowerCase())));
  const isOn = (c: ContentItem) => (cfg.exclusive ? active === c.id : c.enabled);

  function flip(c: ContentItem) {
    if (cfg.exclusive) {
      active = active === c.id ? null : c.id;
      return;
    }
    c.enabled = !c.enabled;
    void api.content.setEnabled(instanceId, kind, c.id, c.enabled);
  }
  function secondary() {
    checking = true;
    setTimeout(() => (checking = false), CHECK_MS);
  }
  function badge(c: ContentItem): { text: string; cls: string } | null {
    if (cfg.exclusive && isOn(c)) return { text: 'Active', cls: 'badge-accent' };
    const s: ContentStatus | undefined = c.status;
    if (!s) return null;
    switch (s.kind) {
      case 'update': return { text: `Update ${s.version}`, cls: 'badge-accent' };
      case 'locked': return { text: 'Locked', cls: 'badge-info' };
      case 'unknown': return { text: 'Unknown', cls: 'badge-warn' };
      case 'outdated': return { text: s.label, cls: 'badge-warn' };
      case 'world': return { text: s.world, cls: '' };
      case 'offline': return { text: 'Offline', cls: 'badge-danger' };
    }
  }
</script>

<div class="page" style="gap: 12px; padding-top: 16px; font-family: var(--font); color: var(--text)">
  <div style="display: flex; flex-wrap: wrap; gap: 8px; align-items: center">
    <SearchField bind:value={search} label="Search" placeholder={searchIn(cfg.noun, items.data?.length)} style="flex: 0 1 280px" />
    <div style="display: flex; gap: 6px; flex-wrap: wrap">
      {#each chips as ch, i (ch.label)}
        <button class="chip" class:on={chip === i} onclick={() => (chip = i)} aria-pressed={chip === i}>{ch.label}<span class="count">{rows.filter(ch.test).length}</span></button>
      {/each}
    </div>
    <div style="flex: 1 1 0"></div>
    <button class="btn" onclick={secondary}>{cfg.secondary}</button>
    {#if cfg.projects}
      <a href={href({ name: 'browse' })} class="btn btn-accent"><Icon name="plus" size={14} sw={2} />{cfg.primary}</a>
    {:else}
      <button class="btn btn-accent"><Icon name="plus" size={14} sw={2} />{cfg.primary}</button>
    {/if}
  </div>
  {#if cfg.hint}<p class="small faint" style="margin: 0">{cfg.hint}</p>{/if}
  <div class="list">
    {#if items.data && !checking}
      {#each shown as c (c.id)}
        {@const on = isOn(c)}
        {@const b = badge(c)}
        <div class="row crow" class:off={cfg.toggles && !on}>
          <span>{#if cfg.toggles}<Toggle {on} label="Enable {c.name}" onchange={() => flip(c)} />{/if}</span>
          <span class="media dim" style="background: {c.art.background}">{c.art.initial ?? ''}</span>
          <a href={cfg.projects ? href({ name: 'project', id: c.id }) : undefined} class="dim" style="min-width: 0; text-decoration: none"><span class="trunc" style="display: block; font-weight: 500">{c.name}</span><span class="trunc small faint" style="display: block">{c.sub}</span></a>
          <span class="c-stat">
            {#if b}<span class="badge {b.cls}">{b.text}</span>{/if}
            {#if c.action}<button class="btn btn-sm" onclick={() => session.play(instanceId)}>{c.action}</button>{/if}
          </span>
          <span class="mono muted trunc">{c.meta}</span>
          <span class="opt">{#if c.source}<span class="src" title={SOURCES[c.source][1]}>{SOURCES[c.source][0]}</span>{/if}</span>
          <button class="btn btn-icon btn-sm btn-ghost" aria-label="More for {c.name}"><Icon name="more" size={14} /></button>
        </div>
      {:else}
        <Empty text={items.data.length ? 'Nothing matches this filter.' : EMPTY[kind]} />
      {/each}
    {:else}
      {#each SKELETON as [a, b], k (k)}
        <div class="row crow" aria-hidden="true">
          <div class="sk" style="width: 30px; height: 18px; border-radius: 9px"></div>
          <div class="sk" style="width: 32px; height: 32px; border-radius: 6px"></div>
          <div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: {a}"></div><div class="sk" style="height: 10px; width: {b}"></div></div>
          <div></div>
          <div class="sk" style="height: 10px; width: 64px"></div>
          <div class="sk opt" style="width: 20px; height: 20px"></div>
          <div></div>
        </div>
      {/each}
    {/if}
  </div>
</div>

<style>
  /* Right side, in order: status (Update, Locked) or action, version text, source, more. */
  .crow{grid-template-columns:30px 32px minmax(0,1fr) 112px 104px 28px 28px}
  @media (max-width:1100px){.crow{grid-template-columns:30px 32px minmax(0,1fr) 112px 96px 28px}.crow .opt{display:none}}
  .c-stat{display:flex;justify-content:flex-end;gap:6px;min-width:0}
  .src{width:20px;height:20px;border-radius:4px;display:grid;place-items:center;font-family:var(--mono);font-size:10px;font-weight:500;background:var(--raise);color:var(--muted)}
  .media{width:32px;height:32px;border-radius:6px;display:grid;place-items:center;font-weight:600;font-size:13px;color:#0a0a0b}
</style>

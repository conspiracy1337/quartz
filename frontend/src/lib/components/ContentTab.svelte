<script lang="ts">
  import { api } from '../api';
  import { managed } from '../api/content';
  import type { AddResult, ContentItem, ContentKind, ContentStatus, Source } from '../api/types';
  import { hasBridge, onEvent } from '../bridge';
  import { searchIn } from '../format';
  import { query } from '../query.svelte';
  import { go, href } from '../router.svelte';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';
  import Menu from '../ui/Menu.svelte';
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
  const isManaged = $derived(managed(kind));
  const info = query(() => (managed(kind) ? api.content.info(instanceId) : Promise.resolve(null)));
  // The shader hint names the shader mod the instance has (Iris or Oculus).
  const hint = $derived(kind === 'shaders' && info.data ? `Only one shader pack runs at a time. ${info.data.shaderMod ? `Uses ${info.data.shaderMod}.` : 'Shaders need Iris or Oculus.'}` : cfg.hint);
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

  async function flip(c: ContentItem) {
    try {
      if (cfg.exclusive) {
        active = active === c.id ? null : c.id;
        await api.content.setActiveShader(instanceId, active ?? '');
        return;
      }
      c.enabled = !c.enabled;
      await api.content.setEnabled(instanceId, kind, c.id, c.enabled);
    } catch (e) {
      c.enabled = !c.enabled;
      session.fail(c.name, e);
    }
  }

  // "Check updates" checks, then opens the review; with updates already found it
  // opens the review straight away.
  const pending = $derived(rows.filter((c) => c.status?.kind === 'update').length);
  const review = () => go({ name: 'instance', id: instanceId, tab: kind, sub: 'updates' });
  async function secondary() {
    if (!isManaged) {
      checking = true;
      setTimeout(() => (checking = false), CHECK_MS);
      return;
    }
    if (pending > 0) return review();
    checking = true;
    try {
      const r = await api.content.review(instanceId, kind, true);
      if (r.updates.length > 0) review();
      else session.notify({ subject: 'Everything is up to date', text: r.locked.length ? `(${r.locked.length} locked)` : '' });
    } catch (e) {
      session.fail('Check updates', e);
    } finally {
      checking = false;
    }
  }

  // The row's "…" menu (composed from the Accounts "…" menu).
  let menu = $state<{ item: ContentItem; x: number; y: number } | null>(null);
  function openMenu(e: MouseEvent, item: ContentItem) {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    menu = { item, x: r.right - 240, y: r.bottom + 4 };
  }
  function act(fn: () => Promise<unknown>, what: string) {
    menu = null;
    fn().catch((e: unknown) => session.fail(what, e));
  }
  async function remove(c: ContentItem) {
    const token = await api.content.remove(instanceId, kind, [c.id]);
    session.notify({ subject: c.name, text: 'was removed', undo: () => void api.content.undo(instanceId, token).catch((e: unknown) => session.fail('Undo', e)) });
  }
  const enabledIds = () => rows.filter((c) => c.enabled).map((c) => c.id);
  function move(c: ContentItem, by: number) {
    const ids = enabledIds();
    const at = ids.indexOf(c.id);
    if (at < 0 || at + by < 0 || at + by >= ids.length) return Promise.resolve();
    ids.splice(at, 1);
    ids.splice(at + by, 0, c.id);
    return api.content.setPackOrder(instanceId, ids);
  }

  // Resource packs: drag an enabled row to reorder (the hint says so). The row
  // follows the pointer and the rows it passes make room; no new elements.
  const reorderable = $derived(kind === 'resourcepacks' && chip === 0 && search.trim() === '');
  let drag = $state<{ id: string; y0: number; dy: number; h: number; from: number; to: number } | null>(null);
  function dragStart(e: PointerEvent, c: ContentItem) {
    if (!reorderable || !c.enabled || e.button !== 0 || (e.target as HTMLElement).closest('button')) return;
    const el = e.currentTarget as HTMLElement;
    const from = enabledIds().indexOf(c.id);
    drag = { id: c.id, y0: e.clientY, dy: 0, h: el.offsetHeight, from, to: from };
    el.setPointerCapture(e.pointerId);
  }
  function dragMove(e: PointerEvent) {
    if (!drag) return;
    const n = enabledIds().length;
    drag.dy = e.clientY - drag.y0;
    drag.to = Math.max(0, Math.min(n - 1, drag.from + Math.round(drag.dy / drag.h)));
  }
  // A drag that moved swallows the click that ends it (the name is a link).
  let dragged = false;
  function swallow(e: MouseEvent) {
    if (!dragged) return;
    dragged = false;
    e.preventDefault();
    e.stopPropagation();
  }
  function dragEnd() {
    if (!drag) return;
    const { id, from, to, dy } = drag;
    dragged = Math.abs(dy) > 4;
    drag = null;
    if (from === to) return;
    const ids = enabledIds();
    ids.splice(from, 1);
    ids.splice(to, 0, id);
    rows = [...ids.map((x) => rows.find((r) => r.id === x)!), ...rows.filter((r) => !r.enabled)];
    api.content.setPackOrder(instanceId, ids).catch((e: unknown) => session.fail('Reorder', e));
  }
  /** How far a row moves while another is dragged past it. */
  function shift(c: ContentItem): number {
    if (!drag || !c.enabled) return 0;
    if (c.id === drag.id) return drag.dy;
    const i = enabledIds().indexOf(c.id);
    if (drag.from < drag.to && i > drag.from && i <= drag.to) return -drag.h;
    if (drag.from > drag.to && i < drag.from && i >= drag.to) return drag.h;
    return 0;
  }

  // Files dropped on the window while this tab shows are added to it; Go keeps
  // the paths, the tab only names where they go.
  $effect(() => {
    if (!managed(kind)) return;
    return onEvent<{ dropId: string; count: number; names: string[] }>('content:dropped', (d) => void add(d.dropId));
  });
  async function add(dropId: string) {
    let world = '';
    if (kind === 'datapacks') {
      const worlds = info.data?.worlds ?? [];
      world = chip > 0 ? (chips[chip]?.label ?? '') : worlds.length === 1 ? worlds[0]! : '';
      if (!world) return session.notify({ tone: 'error', subject: 'Pick a world first', text: 'Choose its chip, then drop the files again.' });
    }
    try {
      announce(await api.content.addDropped(dropId, instanceId, kind, world));
    } catch (e) {
      session.fail('Add files', e);
    }
  }
  function announce(r: AddResult) {
    const [bad] = r.rejected;
    if (r.added.length === 0 && bad) session.notify({ tone: 'error', subject: bad.name, text: `wasn't added. ${bad.reason}.` });
    else if (bad) session.notify({ subject: `Added ${r.added.length} of ${r.added.length + r.rejected.length}`, text: `${bad.name}: ${bad.reason}.` });
    else if (r.added.length) session.notify({ subject: r.added.length === 1 ? r.added[0]! : `${r.added.length} files`, text: 'added' });
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
      case 'problem': return { text: s.label, cls: s.tone === 'info' ? 'badge-info' : `badge-${s.tone}` };
    }
    return null;
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
  {#if hint}<p class="small faint" style="margin: 0">{hint}</p>{/if}
  <div class="list">
    {#if items.data && !checking}
      {#each shown as c (c.id)}
        {@const on = isOn(c)}
        {@const b = badge(c)}
        <div class="row crow" class:off={cfg.toggles && !on} class:drag={drag?.id === c.id} style={drag ? `transform: translateY(${shift(c)}px)` : undefined}
          role={reorderable && c.enabled ? 'listitem' : undefined} onpointerdown={(e) => dragStart(e, c)} onpointermove={dragMove} onpointerup={dragEnd} onpointercancel={dragEnd} onclickcapture={swallow} ondragstart={(e) => reorderable && e.preventDefault()}>
          <span>{#if cfg.toggles}<Toggle {on} label="Enable {c.name}" onchange={() => flip(c)} />{/if}</span>
          <span class="media dim" style="background: {c.art.background}">{c.art.initial ?? ''}</span>
          <a href={cfg.projects && !hasBridge() ? href({ name: 'project', id: c.id }) : undefined} class="dim" style="min-width: 0; text-decoration: none"><span class="trunc" style="display: block; font-weight: 500">{c.name}</span><span class="trunc small faint" style="display: block">{c.sub}</span></a>
          <span class="c-stat">
            {#if b}<span class="badge {b.cls}">{b.text}</span>{/if}
            {#if c.action}<button class="btn btn-sm" onclick={() => session.play(instanceId)}>{c.action}</button>{/if}
          </span>
          <span class="mono muted trunc">{c.meta}</span>
          <span class="opt">{#if c.source}<span class="src" title={SOURCES[c.source][1]}>{SOURCES[c.source][0]}</span>{/if}</span>
          <button class="btn btn-icon btn-sm btn-ghost" aria-label="More for {c.name}" aria-haspopup="menu" disabled={!isManaged} onclick={(e) => openMenu(e, c)}><Icon name="more" size={14} /></button>
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

{#if menu}
  {@const c = menu.item}
  <Menu x={menu.x} y={menu.y} label="More for {c.name}" onclose={() => (menu = null)}>
    {#if kind === 'resourcepacks' && c.enabled}
      <button class="mi" role="menuitem" onclick={() => act(() => move(c, -1), 'Move up')}><Icon name="chevron-up" size={14} /><span>Move up</span><span></span></button>
      <button class="mi" role="menuitem" onclick={() => act(() => move(c, 1), 'Move down')}><Icon name="chevron-down" size={14} /><span>Move down</span><span></span></button>
    {/if}
    {#if c.source && c.source !== 'file'}
      <button class="mi" role="menuitem" onclick={() => act(() => api.content.setLocked(instanceId, c.id, !c.locked), 'Lock version')}><Icon name="lock" size={14} /><span>{c.locked ? 'Unlock version' : 'Lock version'}</span><span></span></button>
    {/if}
    {#if c.project}
      <button class="mi" role="menuitem" onclick={() => act(() => api.app.openUrl(c.project ?? ''), 'Open page')}><Icon name="external" size={14} /><span>Open on {c.source === 'curseforge' ? 'CurseForge' : 'Modrinth'}</span><span></span></button>
    {/if}
    <button class="mi" role="menuitem" onclick={() => act(() => api.content.showInFolder(instanceId, c.id), 'Show in folder')}><Icon name="folder" size={14} /><span>Show in folder</span><span></span></button>
    <button class="mi" role="menuitem" onclick={() => act(() => remove(c), 'Remove')}><Icon name="trash" size={14} /><span>Remove</span><span></span></button>
  </Menu>
{/if}

<style>
  /* Right side, in order: status (Update, Locked) or action, version text, source, more. */
  .crow{grid-template-columns:30px 32px minmax(0,1fr) 112px 104px 28px 28px}
  @media (max-width:1100px){.crow{grid-template-columns:30px 32px minmax(0,1fr) 112px 96px 28px}.crow .opt{display:none}}
  .c-stat{display:flex;justify-content:flex-end;gap:6px;min-width:0}
  .src{width:20px;height:20px;border-radius:4px;display:grid;place-items:center;font-family:var(--mono);font-size:10px;font-weight:500;background:var(--raise);color:var(--muted)}
  .media{width:32px;height:32px;border-radius:6px;display:grid;place-items:center;font-weight:600;font-size:13px;color:#0a0a0b}
  /* Dragging a resource pack: the held row rides above the others, which slide to make room. */
  .crow[role="listitem"]{cursor:grab}
  .crow.drag{position:relative;z-index:1;cursor:grabbing;background:var(--raise);user-select:none}
  @media (prefers-reduced-motion:no-preference){.crow:not(.drag){transition:transform 160ms cubic-bezier(.23,1,.32,1)}}
</style>

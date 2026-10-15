<script lang="ts" module>
  // The sort each tab was left on, kept while the launcher runs.
  const sorts: Partial<Record<string, string>> = {};
</script>

<script lang="ts">
  import { listKeys } from '../listkeys';
  import { api } from '../api';
  import { managed } from '../api/content';
  import type { AddResult, ContentItem, ContentKind, ContentStatus, Source } from '../api/types';
  import { hasBridge, onEvent } from '../bridge';
  import { plural, searchIn, sentenceList } from '../format';
  import { query } from '../query.svelte';
  import { revs } from '../revs.svelte';
  import { projectKindOf } from '../labels';
  import { go, href } from '../router.svelte';
  import { pickRows } from '../selection';
  import { session } from '../session.svelte';
  import Check from '../ui/Check.svelte';
  import Icon from '../ui/Icon.svelte';
  import Menu, { below } from '../ui/Menu.svelte';
  import SearchField from '../ui/SearchField.svelte';
  import SplitButton from '../ui/SplitButton.svelte';
  import Toggle from '../ui/Toggle.svelte';
  import Empty from '../ui/Empty.svelte';

  /** The one list used by Mods, Resource packs, Shaders, Data packs, Worlds and Servers. */
  /** chip names the filter chip to start on (Host's Review opens "Client only"). */
  interface Props { instanceId: string; kind: ContentKind; chip?: string | undefined }
  let { instanceId, kind, chip: startChip }: Props = $props();

  type Chip = { label: string; test: (c: ContentItem) => boolean };
  interface Config { noun: string; secondary: string; primary: string; toggles: boolean; exclusive?: boolean; hint?: string; projects: boolean; chips: (items: ContentItem[]) => Chip[] }
  const all: Chip = { label: 'All', test: () => true };
  const CONFIG: Record<ContentKind, Config> = {
    mods: { noun: 'mods', secondary: 'Check updates', primary: 'Add mods', toggles: true, projects: true,
      // "Client only" (mods a local server leaves out) appears only when there are some.
      chips: (items) => [all, { label: 'Updates', test: (c) => c.status?.kind === 'update' }, { label: 'Disabled', test: (c) => !c.enabled },
        ...(items.some((c) => c.clientOnly) ? [{ label: 'Client only', test: (c: ContentItem) => !!c.clientOnly }] : [])] },
    resourcepacks: { noun: 'resource packs', secondary: 'Check updates', primary: 'Add resource packs', toggles: true, projects: true,
      hint: 'Enabled packs higher in the list override the ones below. Drag a row to reorder.',
      chips: () => [all, { label: 'Enabled', test: (c) => c.enabled }, { label: 'Disabled', test: (c) => !c.enabled }] },
    shaders: { noun: 'shader packs', secondary: 'Check updates', primary: 'Add shaders', toggles: true, exclusive: true, projects: true,
      hint: 'Only one shader pack runs at a time. Uses Iris 1.8.1.', chips: () => [all] },
    datapacks: { noun: 'data packs', secondary: 'Check updates', primary: 'Add data packs', toggles: true, projects: true,
      chips: (items) => [{ label: 'All worlds', test: () => true }, ...[...new Set(items.map((c) => c.group).filter((g) => g !== undefined))].map((g) => ({ label: g, test: (c: ContentItem) => c.group === g }))] },
    worlds: { noun: 'worlds', secondary: 'Open folder', primary: 'Import world', toggles: false, projects: false,
      // Survival and Creative always (as designed), then any other mode a world here has.
      chips: (items) => [all, ...[...new Set(['Survival', 'Creative', ...items.map((c) => c.group).filter((g): g is string => !!g)])].map((g) => ({ label: g, test: (c: ContentItem) => c.group === g }))] },
    servers: { noun: 'servers', secondary: 'Refresh', primary: 'Add server', toggles: false, projects: false,
      chips: () => [all, { label: 'Online', test: (c) => c.online === true }] },
  };
  const SOURCES: Record<Source, [string, string]> = { modrinth: ['M', 'Modrinth'], curseforge: ['C', 'CurseForge'], file: ['F', 'Added from file'] };
  const SKELETON: [string, string][] = [['42%', '22%'], ['30%', '24%'], ['48%', '18%'], ['36%', '20%'], ['56%', '30%'], ['40%', '16%'], ['44%', '26%'], ['32%', '20%']];

  const cfg = $derived(CONFIG[kind]);
  const isManaged = $derived(managed(kind));
  // Worlds and servers have their own row menus and actions (WorldsMenu, ServersMenu).
  const hasMenu = $derived(isManaged || kind === 'worlds' || kind === 'servers');
  const info = query(() => (managed(kind) ? api.content.info(instanceId) : Promise.resolve(null)));
  // The shader hint names the shader mod the instance has (Iris or Oculus).
  // Until its info loads, the app names no mod (the preview keeps the design's sample).
  const hint = $derived(kind === 'shaders' && info.data ? `Only one shader pack runs at a time. ${info.data.shaderMod ? `Uses ${info.data.shaderMod}.` : 'Shaders need Iris or Oculus.'}` : kind === 'shaders' && hasBridge() ? 'Only one shader pack runs at a time.' : cfg.hint);
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
  // The chip to start on is picked once the rows that make it appear are in.
  let started = false;
  $effect(() => {
    if (started || !startChip) return;
    const at = chips.findIndex((c) => c.label === startChip);
    if (at >= 0) {
      chip = at;
      started = true;
    }
  });
  // Search finds names and file names; the sort (Pandora's, Prism's) is kept per tab.
  let sortBy = $state({ ...sorts });
  const sort = $derived(sortBy[kind] ?? '');
  function setSort(v: string) {
    sortBy[kind] = sorts[kind] = v;
    sortMenu = null;
  }
  let sortMenu = $state<{ x: number; y: number } | null>(null);
  const SORTS = $derived([
    { value: '', label: kind === 'resourcepacks' ? 'Load order' : 'Name' },
    { value: 'added', label: 'Recently added' }, { value: 'size', label: 'Size' }, { value: 'enabled', label: 'Enabled first' },
  ]);
  const sortLabel = $derived(SORTS.find((o) => o.value === sort)?.label ?? '');
  const ORDER: Record<string, (a: ContentItem, b: ContentItem) => number> = {
    added: (a, b) => (b.added ?? 0) - (a.added ?? 0),
    size: (a, b) => (b.size ?? 0) - (a.size ?? 0),
    enabled: (a, b) => Number(isOn(b)) - Number(isOn(a)),
  };
  const matches = (c: ContentItem, q: string) => c.name.toLowerCase().includes(q) || !!c.file?.toLowerCase().includes(q);
  const shown = $derived.by(() => {
    const q = search.trim().toLowerCase();
    const out = rows.filter((c) => (chips[chip] ?? all).test(c) && matches(c, q));
    const by = ORDER[sort];
    return by ? out.sort(by) : out;
  });
  const isOn = (c: ContentItem) => (cfg.exclusive ? active === c.id : c.enabled);

  async function flip(c: ContentItem) {
    try {
      if (cfg.exclusive) {
        active = active === c.id ? null : c.id;
        await api.content.setActiveShader(instanceId, active ?? '');
        return;
      }
      const need = c.enabled ? await needs([c.id]) : '';
      c.enabled = !c.enabled;
      await api.content.setEnabled(instanceId, kind, c.id, c.enabled);
      if (need) session.notify({ subject: c.name, text: `is off. ${need}.` });
    } catch (e) {
      c.enabled = !c.enabled;
      session.fail(c.name, e);
    }
  }

  /** "Sodium Extra needs it": what turning these mods off or removing them leaves unmet (Prism asks). */
  async function needs(ids: string[]): Promise<string> {
    if (kind !== 'mods') return '';
    const names = await api.content.dependents(instanceId, ids).catch(() => [] as string[]);
    if (!names.length) return '';
    return `${sentenceList(names.length > 3 ? [...names.slice(0, 2), `${names.length - 2} more`] : names)} ${names.length === 1 ? 'needs' : 'need'} ${ids.length === 1 ? 'it' : 'them'}`;
  }

  // Selecting rows (the update review's checkbox, over the row's icon) for bulk
  // actions: Shift-click picks a range, Ctrl+A everything shown, Esc clears.
  const selectable = $derived(isManaged);
  let picked = $state<Record<string, boolean>>({});
  let anchor: string | null = null;
  let shiftHeld = false;
  const selected = $derived(rows.filter((c) => picked[c.id]));
  $effect(() => {
    void kind;
    void instanceId;
    picked = {};
  });
  function pick(c: ContentItem, on: boolean) {
    anchor = pickRows(picked, shown.map((x) => x.id), c.id, on, anchor, shiftHeld);
  }
  $effect(() => {
    if (!selected.length) return;
    const esc = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !session.overlay) picked = {};
    };
    window.addEventListener('keydown', esc);
    return () => window.removeEventListener('keydown', esc);
  });
  const updatable = $derived(selected.filter((c) => c.status?.kind === 'update'));
  async function bulkSet(on: boolean, list = selected) {
    const ids = list.map((c) => c.id);
    try {
      const need = on ? '' : await needs(ids);
      await api.content.setEnabledMany(instanceId, kind, ids, on);
      picked = {};
      if (need) session.notify({ subject: plural(ids.length, cfg.noun.replace(/s$/, '')), text: `turned off. ${need}.` });
    } catch (e) {
      session.fail(on ? 'Turn on' : 'Turn off', e);
    }
  }
  async function bulkRemove() {
    const ids = selected.map((c) => c.id);
    try {
      const need = await needs(ids);
      const token = await api.content.remove(instanceId, kind, ids);
      picked = {};
      session.notify({ subject: plural(ids.length, cfg.noun.replace(/s$/, '')), text: need ? `removed. ${need}.` : 'removed', undo: () => void api.content.undo(instanceId, token).catch((e: unknown) => session.fail('Undo', e)) });
    } catch (e) {
      session.fail('Remove', e);
    }
  }
  function bulkUpdate() {
    session.updatePick = new Set(updatable.map((c) => c.id));
    picked = {};
    review();
  }

  // "Check updates" checks, then opens the review; with updates already found it
  // opens the review straight away.
  const pending = $derived(rows.filter((c) => c.status?.kind === 'update').length);
  const review = () => go({ name: 'instance', id: instanceId, tab: kind, sub: 'updates' });
  async function secondary() {
    if (kind === 'worlds') return api.worlds.openFolder(instanceId).catch((e: unknown) => session.fail('Open folder', e));
    if (kind === 'servers') {
      checking = true;
      try {
        await api.servers.list(instanceId, true);
        revs.bump('servers');
      } catch (e) {
        session.fail('Refresh', e);
      } finally {
        checking = false;
      }
      return;
    }
    if (!isManaged) return;
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

  // Import world and Add server.
  async function primary() {
    if (kind === 'servers') return session.open({ kind: 'server', instanceId });
    try {
      const name = await api.worlds.import(instanceId);
      if (name) session.notify({ subject: name, text: 'added' });
    } catch (e) {
      session.fail('Import world', e);
    }
  }

  // A row's Play or Join opens the game straight into that world or server.
  function rowAction(c: ContentItem) {
    if (kind === 'worlds') return session.play(instanceId, '', { kind: 'world', target: c.id });
    const address = api.servers.entry(instanceId, c.id)?.address;
    session.play(instanceId, '', address ? { kind: 'server', target: address } : undefined);
  }

  async function removeWorld(c: ContentItem) {
    const token = await api.worlds.remove(instanceId, c.id);
    revs.bump('worlds');
    session.notify({ subject: c.name, text: 'was deleted', undo: () => void api.worlds.undo(instanceId, token).then(() => revs.bump('worlds')).catch((e: unknown) => session.fail('Undo', e)) });
  }
  async function exportWorld(c: ContentItem) {
    if (await api.worlds.export(instanceId, c.id)) session.notify({ subject: c.name, text: 'was exported' });
  }
  async function removeServer(c: ContentItem) {
    const e = api.servers.entry(instanceId, c.id);
    if (!e) return;
    const token = await api.servers.remove(instanceId, e.index, e.address);
    revs.bump('servers');
    session.notify({ subject: c.name, text: 'was removed', undo: () => void api.servers.undo(instanceId, token).then(() => revs.bump('servers')).catch((err: unknown) => session.fail('Undo', err)) });
  }
  async function moveServer(c: ContentItem, by: number) {
    const e = api.servers.entry(instanceId, c.id);
    if (!e) return;
    await api.servers.move(instanceId, e.index, e.address, by);
    revs.bump('servers');
  }

  // The row's "…" menu (composed from the Accounts "…" menu).
  let menu = $state<{ item: ContentItem; x: number; y: number } | null>(null);
  const openMenu = (e: MouseEvent, item: ContentItem) => (menu = { item, ...below(e) });
  // Act first: closing the menu ends the block its item comes from.
  function act(fn: () => Promise<unknown>, what: string) {
    fn().catch((e: unknown) => session.fail(what, e));
    menu = null;
  }
  async function remove(c: ContentItem) {
    const need = await needs([c.id]);
    const token = await api.content.remove(instanceId, kind, [c.id]);
    session.notify({ subject: c.name, text: need ? `was removed. ${need}.` : 'was removed', undo: () => void api.content.undo(instanceId, token).catch((e: unknown) => session.fail('Undo', e)) });
  }
  /** A missing dependency's row menu installs it (XMCL, Prism): find its project, then the usual Install dialog. */
  async function installDep(depId: string) {
    const ref = await api.content.findMod(depId);
    session.open({ kind: 'install', ref, source: ref.startsWith('cf:') ? 'curseforge' : 'modrinth', instanceId, content: 'mods' });
  }
  const unusedLibs = $derived(kind === 'mods' ? rows.filter((c) => c.enabled && c.status?.kind === 'problem' && c.status.problem === 'library') : []);
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
  const reorderable = $derived(kind === 'resourcepacks' && chip === 0 && search.trim() === '' && sort === '');
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
    if (!managed(kind) && kind !== 'worlds') return;
    return onEvent<{ dropId: string; count: number; names: string[] }>('content:dropped', (d) => void add(d.dropId));
  });
  async function add(dropId: string) {
    if (kind === 'worlds') {
      try {
        const added = await api.worlds.importDropped(dropId, instanceId);
        revs.bump('worlds');
        if (added.length) session.notify({ subject: added.length === 1 ? added[0]! : `${added.length} worlds`, text: 'added' });
      } catch (e) {
        session.fail('Import world', e);
      }
      return;
    }
    const world = worldFor('drop the files again');
    if (world === null) return;
    try {
      announce(await api.content.addDropped(dropId, instanceId, kind, world));
    } catch (e) {
      session.fail('Add files', e);
    }
  }
  /** The world added data packs go to: the chosen chip's, or the only one; null (and a toast) when unclear. */
  function worldFor(retry: string): string | null {
    if (kind !== 'datapacks') return '';
    const worlds = info.data?.worlds ?? [];
    const world = chip > 0 ? (chips[chip]?.label ?? '') : worlds.length === 1 ? worlds[0]! : '';
    if (!world) session.notify({ tone: 'error', subject: 'Pick a world first', text: `Choose its chip, then ${retry}.` });
    return world || null;
  }
  // Add's ▾: files from disk (for when dragging in doesn't work), and on Mods
  // turning off library mods nothing uses.
  let addMenu = $state<{ x: number; y: number } | null>(null);
  const openAddMenu = (e: MouseEvent) => (addMenu = below(e));
  async function addFiles() {
    addMenu = null;
    const world = worldFor('add the files again');
    if (world === null) return;
    try {
      announce(await api.content.addFiles(instanceId, kind, world));
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
    <SearchField bind:value={search} label="Search" placeholder={searchIn(cfg.noun, items.data?.length)} style="flex: 0 1 {isManaged ? 260 : 280}px" />
    <div style="display: flex; gap: 6px; flex-wrap: wrap">
      {#each chips as ch, i (ch.label)}
        <button class="chip" class:on={chip === i} onclick={() => (chip = i)} aria-pressed={chip === i}>{ch.label}<span class="count">{rows.filter(ch.test).length}</span></button>
      {/each}
    </div>
    <div style="flex: 1 1 0"></div>
    {#if isManaged}<button class="btn btn-icon" aria-label="Sort {cfg.noun}: {sortLabel}" title="Sort: {sortLabel}" aria-haspopup="menu" aria-expanded={!!sortMenu} onclick={(e) => (sortMenu = below(e))}><Icon name="up-down" size={14} /></button>{/if}
    <button class="btn" onclick={secondary}>{cfg.secondary}</button>
    {#if cfg.projects}
      <SplitButton accent href={href({ name: 'browse', type: projectKindOf(kind), instance: instanceId })} menuLabel="More ways to add {cfg.noun}" expanded={!!addMenu} onmenu={openAddMenu}><Icon name="plus" size={14} sw={2} />{cfg.primary}</SplitButton>
    {:else}
      <button class="btn btn-accent" onclick={primary}><Icon name="plus" size={14} sw={2} />{cfg.primary}</button>
    {/if}
  </div>
  {#if hint}<p class="small faint" style="margin: 0">{hint}</p>{/if}
  <div class="list" class:selecting={selected.length > 0} use:listKeys>
    {#if items.data && !checking}
      {#each shown as c (c.id)}
        {@const on = isOn(c)}
        {@const b = badge(c)}
        <div class="row crow" class:off={cfg.toggles && !on} class:drag={drag?.id === c.id} style={drag ? `transform: translateY(${shift(c)}px)` : undefined}
          role={reorderable && c.enabled ? 'listitem' : undefined} onpointerdown={(e) => dragStart(e, c)} onpointermove={dragMove} onpointerup={dragEnd} onpointercancel={dragEnd} onclickcapture={swallow} ondragstart={(e) => reorderable && e.preventDefault()}>
          <span>{#if cfg.toggles}<Toggle {on} label="Enable {c.name}" onchange={() => flip(c)} />{/if}</span>
          <span class="pickcell">
            <span class="media dim" style="background: {c.art.background}">{c.art.initial ?? ''}</span>
            {#if selectable}<span class="pick" onclickcapture={(e) => (shiftHeld = e.shiftKey)}><Check on={!!picked[c.id]} label="Select {c.name}" onchange={(v) => pick(c, v)} /></span>{/if}
          </span>
          <a href={cfg.projects && c.ref ? href({ name: 'project', id: c.ref }) : undefined} class="dim" style="min-width: 0; text-decoration: none"><span class="trunc" style="display: block; font-weight: 500">{c.name}</span><span class="trunc small faint" style="display: block">{c.sub}</span></a>
          <span class="c-stat">
            {#if b}<span class="badge {b.cls}">{b.text}</span>{/if}
            {#if c.action}<button class="btn btn-sm" onclick={() => rowAction(c)}>{c.action}</button>{/if}
          </span>
          <span class="mono muted trunc">{c.meta}</span>
          <span class="opt">{#if c.source}<span class="src" title={SOURCES[c.source][1]}>{SOURCES[c.source][0]}</span>{/if}</span>
          <button class="btn btn-icon btn-sm btn-ghost" aria-label="More for {c.name}" aria-haspopup="menu" disabled={!hasMenu} onclick={(e) => openMenu(e, c)}><Icon name="more" size={14} /></button>
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
  {#if selected.length}
    <!-- Composed from the update review's footer: what is selected, and what to do with it. -->
    <div class="bulk">
      <span class="small faint tnum" style="margin-right: auto">{selected.length} selected</span>
      <button class="btn" onclick={() => (picked = {})}>Cancel</button>
      <button class="btn" onclick={bulkRemove}>Remove</button>
      {#if cfg.toggles}
        <button class="btn" onclick={() => bulkSet(false)}>Turn off</button>
        {#if !cfg.exclusive}<button class="btn" onclick={() => bulkSet(true)}>Turn on</button>{/if}
      {/if}
      {#if updatable.length}<button class="btn btn-accent" onclick={bulkUpdate}>Update {updatable.length}</button>{/if}
    </div>
  {/if}
</div>

{#if sortMenu}
  <Menu x={sortMenu.x} y={sortMenu.y} label="Sort {cfg.noun}" title="Sort by" onclose={() => (sortMenu = null)}>
    {#each SORTS as o (o.value)}
      <button class="mi" role="menuitem" aria-current={o.value === sort} onclick={() => setSort(o.value)}>{#if o.value === sort}<Icon name="check" size={14} />{:else}<span></span>{/if}<span>{o.label}</span><span></span></button>
    {/each}
  </Menu>
{/if}

{#if addMenu}
  <Menu x={addMenu.x} y={addMenu.y} label="More ways to add {cfg.noun}" onclose={() => (addMenu = null)}>
    <button class="mi" role="menuitem" onclick={addFiles}><Icon name="file" size={14} /><span>From file…</span><span></span></button>
    {#if unusedLibs.length}
      <button class="mi" role="menuitem" onclick={() => { addMenu = null; void bulkSet(false, unusedLibs); }}><Icon name="stop" size={14} /><span>Turn off unused libraries</span><span class="small faint">{unusedLibs.length}</span></button>
    {/if}
  </Menu>
{/if}

{#if menu}
  {@const c = menu.item}
  <Menu x={menu.x} y={menu.y} label="More for {c.name}" onclose={() => (menu = null)}>
    {#if kind === 'worlds'}
      <!-- WorldsMenu.dc.html: Play stays on the row. -->
      <button class="mi" role="menuitem" onclick={() => act(async () => session.open({ kind: 'rename-world', instanceId, folder: c.id, name: c.name }), 'Rename')}><Icon name="file" size={14} /><span>Rename</span><span></span></button>
      <button class="mi" role="menuitem" onclick={() => act(async () => session.copy(await api.worlds.seed(instanceId, c.id), 'Seed'), 'Copy seed')}><Icon name="link" size={14} /><span>Copy seed</span><span></span></button>
      <button class="mi" role="menuitem" onclick={() => act(() => exportWorld(c), 'Export')}><Icon name="download" size={14} /><span>Export as .zip</span><span></span></button>
      <button class="mi" role="menuitem" onclick={() => act(async () => session.open({ kind: 'copy-world', instanceId, folder: c.id, name: c.name, sub: `${c.sub.split(', ').at(-1)}, ${c.meta}` }), 'Copy')}><Icon name="library" size={14} /><span>Copy to instance…</span><span></span></button>
      <button class="mi" role="menuitem" onclick={() => act(() => api.worlds.showInFolder(instanceId, c.id), 'Show in folder')}><Icon name="folder" size={14} /><span>Show in folder</span><span></span></button>
      <button class="mi" role="menuitem" onclick={() => act(() => removeWorld(c), 'Delete')}><Icon name="trash" size={14} /><span>Delete</span><span></span></button>
    {:else if kind === 'servers'}
      <!-- ServersMenu.dc.html -->
      {@const entry = api.servers.entry(instanceId, c.id)}
      <button class="mi" role="menuitem" onclick={() => act(async () => rowAction(c), 'Join')}><Icon name="play" size={14} /><span>Join</span><span></span></button>
      <button class="mi" role="menuitem" onclick={() => act(async () => session.open({ kind: 'server', instanceId, ...(entry ? { entry } : {}) }), 'Edit')}><Icon name="file" size={14} /><span>Edit</span><span></span></button>
      <button class="mi" role="menuitem" onclick={() => act(() => session.copy(entry?.address ?? '', 'Address'), 'Copy address')}><Icon name="link" size={14} /><span>Copy address</span><span></span></button>
      <div class="divider" style="margin: 4px 0"></div>
      <button class="mi" role="menuitem" onclick={() => act(() => moveServer(c, -1), 'Move up')}><Icon name="chevron-up" size={14} /><span>Move up</span><span></span></button>
      <button class="mi" role="menuitem" onclick={() => act(() => moveServer(c, 1), 'Move down')}><Icon name="chevron-down" size={14} /><span>Move down</span><span></span></button>
      <div class="divider" style="margin: 4px 0"></div>
      <button class="mi" role="menuitem" onclick={() => act(() => removeServer(c), 'Remove')}><Icon name="trash" size={14} /><span>Remove</span><span></span></button>
    {:else}
    {#if kind === 'resourcepacks' && c.enabled}
      <button class="mi" role="menuitem" onclick={() => act(() => move(c, -1), 'Move up')}><Icon name="chevron-up" size={14} /><span>Move up</span><span></span></button>
      <button class="mi" role="menuitem" onclick={() => act(() => move(c, 1), 'Move down')}><Icon name="chevron-down" size={14} /><span>Move down</span><span></span></button>
    {/if}
    {#if c.status?.kind === 'problem' && c.status.problem === 'missing' && c.status.depId}
      {@const dep = c.status.depId}
      <button class="mi" role="menuitem" onclick={() => act(() => installDep(dep), 'Install')}><Icon name="download" size={14} /><span>Install {c.status.label.replace(/^Needs /, '')}</span><span></span></button>
    {/if}
    {#if cfg.projects && c.ref}
      {@const ref = c.ref}
      <button class="mi" role="menuitem" onclick={() => act(async () => { session.browseTarget = instanceId; go({ name: 'project', id: ref, tab: 'versions' }); }, 'Change version')}><Icon name="refresh" size={14} /><span>Change version…</span><span></span></button>
    {/if}
    {#if c.source && c.source !== 'file'}
      <button class="mi" role="menuitem" onclick={() => act(() => api.content.setLocked(instanceId, c.id, !c.locked), 'Lock version')}><Icon name="lock" size={14} /><span>{c.locked ? 'Unlock version' : 'Lock version'}</span><span></span></button>
    {/if}
    {#if c.project}
      <button class="mi" role="menuitem" onclick={() => act(() => api.app.openUrl(c.project ?? ''), 'Open page')}><Icon name="external" size={14} /><span>Open on {c.source === 'curseforge' ? 'CurseForge' : 'Modrinth'}</span><span></span></button>
    {/if}
    <button class="mi" role="menuitem" onclick={() => act(() => api.content.showInFolder(instanceId, c.id), 'Show in folder')}><Icon name="folder" size={14} /><span>Show in folder</span><span></span></button>
    <button class="mi" role="menuitem" onclick={() => act(() => remove(c), 'Remove')}><Icon name="trash" size={14} /><span>Remove</span><span></span></button>
    {/if}
  </Menu>
{/if}

<style>
  /* Right side, in order: status (Update, Locked) or action, version text, source, more. */
  .crow{grid-template-columns:30px 32px minmax(0,1fr) 112px 104px 28px 28px}
  @media (max-width:1100px){.crow{grid-template-columns:30px 32px minmax(0,1fr) 112px 96px 28px}.crow .opt{display:none}}
  .c-stat{display:flex;justify-content:flex-end;gap:6px;min-width:0}
  .src{width:20px;height:20px;border-radius:4px;display:grid;place-items:center;font-family:var(--mono);font-size:10px;font-weight:500;background:var(--raise);color:var(--muted)}
  .media{width:32px;height:32px;border-radius:6px;display:grid;place-items:center;font-weight:600;font-size:13px;color:#0a0a0b}
  /* The selection checkbox sits over the icon: shown on hover or focus, and on every row once something is selected. */
  .pickcell{position:relative;width:32px;height:32px}
  .pick{position:absolute;inset:0;display:grid;place-items:center;border-radius:6px;background:var(--raise);opacity:0}
  .crow:hover .pick,.pick:focus-within,.selecting .pick{opacity:1}
  .bulk{position:sticky;bottom:0;display:flex;justify-content:flex-end;gap:8px;align-items:center;padding:12px 0;background:var(--bg)}
  /* Dragging a resource pack: the held row rides above the others, which slide to make room. */
  .crow[role="listitem"]{cursor:grab}
  .crow.drag{position:relative;z-index:1;cursor:grabbing;background:var(--raise);user-select:none}
  @media (prefers-reduced-motion:no-preference){.crow:not(.drag){transition:transform 160ms cubic-bezier(.23,1,.32,1)}}
</style>

<script lang="ts" module>
  import type { Sort } from '../sort';

  type Key = 'name' | 'version' | 'players' | 'ping';
  // The sort and Online toggle each table (Home, an instance's tab) was left on, kept while the launcher runs.
  const sorts: Partial<Record<string, Sort<Key>>> = {};
  const onlineOnly: Partial<Record<string, boolean>> = {};
</script>

<script lang="ts">
  import { api } from '../api';
  import { players, plural } from '../format';
  import { listKeys } from '../listkeys';
  import { query, SKELETON } from '../query.svelte';
  import { noTextPick, Picker } from '../selection.svelte';
  import type { ServerRow, ServerScope } from '../servers';
  import { session } from '../session.svelte';
  import { byStats, names, nextSort } from '../sort';
  import Check from '../ui/Check.svelte';
  import Empty from '../ui/Empty.svelte';
  import Icon from '../ui/Icon.svelte';
  import Menu, { below } from '../ui/Menu.svelte';
  import ServerPing from '../ui/ServerPing.svelte';
  import ServerVersion from '../ui/ServerVersion.svelte';
  import SortTitle from '../ui/SortTitle.svelte';
  import SplitButton from '../ui/SplitButton.svelte';
  import Tile from '../ui/Tile.svelte';

  /**
   * The one servers table, on Home (every instance's servers) and an instance's Servers tab:
   * column titles that sort, Online above Join, select boxes for bulk Remove, and the row's ⋮.
   * rows is undefined until they load (skeleton rows); empty is the text with no rows at all.
   */
  interface Props { rows: ServerRow[] | undefined; scope: ServerScope; skeleton: number; empty: string }
  let { rows, scope, skeleton, empty }: Props = $props();

  // svelte-ignore state_referenced_locally
  const key = scope.key;
  let sort = $state(sorts[key] ?? null);
  let online = $state(onlineOnly[key] ?? false);
  function sortOn(k: Key) {
    sort = sorts[key] = nextSort(sort, k);
  }
  function toggleOnline() {
    online = onlineOnly[key] = !online;
  }

  const COLUMNS: [Key, string, string][] = [['name', 'Name', ''], ['version', 'Version', 'end c-ver'], ['players', 'Players', 'end'], ['ping', 'Ping', 'end']];
  const COMPARE: Record<Key, (a: ServerRow, b: ServerRow) => number> = {
    name: (a, b) => names.compare(a.name, b.name),
    version: (a, b) => byStats.version(a.stats, b.stats),
    players: (a, b) => byStats.players(a.stats, b.stats),
    ping: (a, b) => byStats.ping(a.stats, b.stats),
  };
  // Without a sort, rows stay as they came (an instance's list as saved, Home's online first).
  const shown = $derived.by(() => {
    const out = (rows ?? []).filter((r) => !online || r.stats?.online);
    if (!sort) return out;
    const { key: k, dir } = sort;
    return out.sort((a, b) => COMPARE[k](a, b) * dir);
  });

  const picker = new Picker(() => shown.map((r) => r.id));
  const selected = $derived((rows ?? []).filter((r) => picker.picked[r.id]));
  $effect(() => picker.escape());

  let refreshing = $state(false);
  async function refresh() {
    refreshing = true;
    try {
      await scope.refresh();
    } catch (e) {
      session.fail('Refresh', e);
    } finally {
      refreshing = false;
    }
  }

  /** "Hypixel was removed" for one row, "3 servers removed" for several. */
  const gone = (list: ServerRow[]) =>
    list.length === 1 ? { subject: list[0]!.name, text: 'was removed' } : { subject: plural(list.length, 'server'), text: 'removed' };
  async function remove(list: ServerRow[]) {
    const undo = await scope.remove(list);
    picker.clear();
    if (undo) session.notify({ ...gone(list), undo: () => void undo().catch((e: unknown) => session.fail('Undo', e)) });
  }

  // The row's ⋮ menu, and Join's ▾ (another instance that has the server).
  let menu = $state<{ row: ServerRow; x: number; y: number } | null>(null);
  let joinMenu = $state<{ row: ServerRow; x: number; y: number } | null>(null);
  // svelte-ignore state_referenced_locally
  const instances = scope.pick ? query(() => api.instances.list(), 'instances.list') : null;
  // Act first: closing the menu ends the block its item comes from.
  function act(fn: () => unknown, what: string) {
    Promise.resolve().then(fn).catch((e: unknown) => session.fail(what, e));
    menu = null;
  }
  function joinWith(r: ServerRow, instanceId: string) {
    joinMenu = null;
    scope.join(r, instanceId);
  }
</script>

<div class="list list-fill srv-list" use:listKeys>
  <div class="row row-head srv">
    <span></span>
    {#each COLUMNS as [k, label, cls] (k)}
      <span class={cls}><SortTitle {label} dir={sort?.key === k ? sort.dir : 0} onclick={() => sortOn(k)} /></span>
    {/each}
    <span class="end"><button class="chip tog" class:on={online} aria-pressed={online} onclick={toggleOnline}>Online</button></span>
    <span class="cc"><Check on={picker.all} label="Select every shown server" onchange={(v) => picker.pickAll(v)} /></span>
    <span class="cc"><button class="btn-bare" aria-label="Refresh" title="Refresh" disabled={refreshing} onclick={refresh}><Icon name="refresh" size={14} sw={1.8} class={refreshing ? 'spin' : ''} /></button></span>
  </div>
  {#if rows && !refreshing}
    {#each shown as r (r.id)}
      <!-- Ctrl/Shift-click is a mouse shortcut for the row's select box, which keyboards use. -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="row srv" class:sel={!!picker.picked[r.id]} onclickcapture={(e) => picker.click(e, r.id)} onmousedown={noTextPick}>
        <span class="media" style="background: {r.art.background}"></span>
        <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{r.name}</span><span class="trunc small faint mono" style="display: block">{r.address}</span></span>
        <span class="end c-ver">{#if r.stats}<ServerVersion server={r.stats} />{/if}</span>
        <span class="end mono muted tnum trunc">{players(r.stats?.players)}</span>
        <span class="end"><ServerPing ms={r.stats?.pingMs} /></span>
        <!-- Only a server that answered can be joined from here; the ⋮ still has Join. -->
        <span class="end">
          {#if r.stats?.online}
            {#if scope.pick}
              <SplitButton size="sm" accent onclick={() => scope.join(r)} onmenu={(e) => (joinMenu = { row: r, ...below(e) })} expanded={joinMenu?.row.id === r.id} menuLabel="Join {r.name} with another instance">Join</SplitButton>
            {:else}
              <SplitButton size="sm" accent style="width: var(--tog-w)" onclick={() => scope.join(r)}>Join</SplitButton>
            {/if}
          {/if}
        </span>
        <span class="cc"><Check on={!!picker.picked[r.id]} label="Select {r.name}" onchange={(v) => picker.pick(r.id, v)} /></span>
        <button class="btn btn-icon btn-sm btn-ghost" aria-label="More for {r.name}" aria-haspopup="menu" onclick={(e) => (menu = { row: r, ...below(e) })}><Icon name="more" size={14} /></button>
      </div>
    {:else}
      <Empty text={rows.length ? 'Nothing matches this filter.' : empty} />
    {/each}
  {:else}
    {#each { length: skeleton } as _, k (k)}
      {@const [a, w] = SKELETON[k % SKELETON.length]!}
      <div class="row srv" aria-hidden="true">
        <div class="sk" style="width: 32px; height: 32px; border-radius: 6px"></div>
        <div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: {a}"></div><div class="sk" style="height: 10px; width: {w}"></div></div>
        <div class="end c-ver"><div class="sk" style="height: 20px; width: 52px"></div></div>
        <div class="end"><div class="sk" style="height: 10px; width: 56px"></div></div>
        <div class="end"><div class="sk" style="height: 10px; width: 64px"></div></div>
        <div class="end"><div class="sk" style="height: 28px; width: var(--tog-w)"></div></div>
        <div class="cc"><div class="sk" style="width: 16px; height: 16px"></div></div>
        <div></div>
      </div>
    {/each}
  {/if}
</div>
{#if selected.length}
  <!-- Composed from the update review's footer: what is selected, and what to do with it. -->
  <div class="bulk">
    <span class="small faint tnum" style="margin-right: auto">{selected.length} selected</span>
    <button class="btn" onclick={() => picker.clear()}>Cancel</button>
    <button class="btn" onclick={() => remove(selected).catch((e: unknown) => session.fail('Remove', e))}>Remove</button>
  </div>
{/if}

{#if menu}
  {@const r = menu.row}
  <!-- ServersMenu.dc.html -->
  <Menu x={menu.x} y={menu.y} label="More for {r.name}" onclose={() => (menu = null)}>
    <button class="mi" role="menuitem" onclick={() => act(() => scope.join(r), 'Join')}><Icon name="play" size={14} /><span>Join</span><span></span></button>
    {#if scope.edit}
      {@const edit = scope.edit}
      <button class="mi" role="menuitem" onclick={() => act(() => edit(r), 'Edit')}><Icon name="file" size={14} /><span>Edit</span><span></span></button>
    {/if}
    <button class="mi" role="menuitem" onclick={() => act(() => session.copy(r.address, 'Address'), 'Copy address')}><Icon name="link" size={14} /><span>Copy address</span><span></span></button>
    {#if scope.move}
      {@const move = scope.move}
      <div class="divider" style="margin: 4px 0"></div>
      <button class="mi" role="menuitem" onclick={() => act(() => move(r, -1), 'Move up')}><Icon name="chevron-up" size={14} /><span>Move up</span><span></span></button>
      <button class="mi" role="menuitem" onclick={() => act(() => move(r, 1), 'Move down')}><Icon name="chevron-down" size={14} /><span>Move down</span><span></span></button>
    {/if}
    <div class="divider" style="margin: 4px 0"></div>
    <button class="mi" role="menuitem" onclick={() => act(() => remove([r]), 'Remove')}><Icon name="trash" size={14} /><span>Remove</span><span></span></button>
  </Menu>
{/if}

{#if joinMenu}
  {@const r = joinMenu.row}
  <Menu x={joinMenu.x} y={joinMenu.y} label="Join {r.name} with" title="Join {r.name} with" onclose={() => (joinMenu = null)}>
    {#each r.instances as id, j (id)}
      {@const i = instances?.data?.find((x) => x.id === id)}
      {#if i}
        <button class="mi" role="menuitem" onclick={() => joinWith(r, id)}><Tile art={i.art} icon={i.icon} loader={i.loader?.kind ?? ''} size="sm" /><span class="trunc">{i.name}</span><span class="small faint">{j === 0 ? 'Last used' : i.minecraft}</span></button>
      {/if}
    {/each}
  </Menu>
{/if}

<style>
  /* Icon, name, then version, players and ping, Join, select, more. Players fits "38,412 / 200,000".
     The columns follow the table's own width, so it fits Home's column and an instance page alike. */
  .srv-list{container-type:inline-size;--tog-w:60px}
  .srv{grid-template-columns:32px minmax(0,1fr) 120px 128px 120px 72px 28px 28px;column-gap:24px}
  @container (max-width:980px){.srv{grid-template-columns:32px minmax(0,1fr) 96px 120px 96px 72px 28px 28px;column-gap:16px}}
  @container (max-width:640px){.srv{grid-template-columns:32px minmax(0,1fr) 120px 96px 72px 28px 28px}.srv .c-ver{display:none}}
  /* The Online toggle and a Join without ▾ share one width, so they line up in their column. */
  .tog{width:var(--tog-w);justify-content:center}
</style>

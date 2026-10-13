<script lang="ts">
  import { api } from '$lib/api';
  import { Day, clock, searchIn } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import { revs } from '$lib/revs.svelte';
  import { href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import SearchField from '$lib/ui/SearchField.svelte';
  import Empty from '$lib/ui/Empty.svelte';

  interface Props { instanceId: string }
  let { instanceId }: Props = $props();

  const WEEK = 7 * 86_400_000;
  const shots = query(() => api.screenshots.list(instanceId));
  let search = $state('');
  let filter = $state<string>('all');

  const list = $derived(shots.data ?? []);
  const worlds = $derived([...new Set(list.map((s) => s.world).filter((w) => w !== ''))]);
  const chips = $derived([
    { key: 'all', label: 'All' },
    { key: 'week', label: 'This week' },
    ...worlds.map((w) => ({ key: `world:${w}`, label: w })),
  ]);
  const match = (s: (typeof list)[number], key: string) => key === 'all' || (key === 'week' ? Date.now() - s.takenAt < WEEK : `world:${s.world}` === key);
  const q = $derived(search.trim().toLowerCase());
  const found = (s: (typeof list)[number]) => !q || Day(s.takenAt).toLowerCase().includes(q) || s.world.toLowerCase().includes(q);
  const shown = $derived(list.filter((s) => match(s, filter) && found(s)));
  // The empty line names what hides the screenshots.
  const emptyText = $derived(!list.length ? 'No screenshots yet. Press F2 in the game to take one.' : q ? 'No screenshots match this search.' : filter === 'week' ? 'No screenshots this week.' : 'No screenshots from this world.');
  const when = (t: number) => `${Day(t)} ${clock(t)}`;

  // Select: tiles toggle instead of opening, then the picked ones go to the trash together.
  let selecting = $state(false);
  let picked = $state<string[]>([]);
  function toggle(e: MouseEvent, id: string) {
    if (!selecting) return;
    e.preventDefault();
    picked = picked.includes(id) ? picked.filter((p) => p !== id) : [...picked, id];
  }
  function endSelect() {
    selecting = false;
    picked = [];
  }
  async function trashPicked() {
    const names = picked;
    if (!names.length) return;
    try {
      const token = await api.screenshots.trash(instanceId, names);
      endSelect();
      revs.bump('screenshots');
      session.notify({ tone: 'trash', subject: names.length === 1 ? names[0]! : `${names.length} screenshots`, text: 'to the trash',
        undo: () => void api.screenshots.undo(instanceId, token).then(() => revs.bump('screenshots')).catch((e: unknown) => session.fail('Undo', e)) });
    } catch (e) {
      session.fail('Move to trash', e);
    }
  }
  const openFolder = () => api.screenshots.openFolder(instanceId).catch((e: unknown) => session.fail('Open folder', e));
</script>

<div class="scroll">
  <div class="page" style="gap: 12px; padding-top: 16px">
    <div style="display: flex; flex-wrap: wrap; gap: 8px; align-items: center">
      <SearchField bind:value={search} label="Search screenshots" placeholder={searchIn('screenshots', shots.data?.length)} style="flex: 0 1 280px" />
      <div style="display: flex; gap: 6px; flex-wrap: wrap">
        {#each chips as c (c.key)}<button class="chip" class:on={filter === c.key} onclick={() => (filter = c.key)}>{c.label}<span class="count">{list.filter((s) => match(s, c.key)).length}</span></button>{/each}
      </div>
      <div style="flex: 1 1 0"></div>
      {#if selecting}
        <button class="btn" onclick={endSelect}>Cancel</button>
        <button class="btn btn-danger" disabled={!picked.length} onclick={trashPicked}>Move {picked.length || ''} to trash</button>
      {:else}
        <button class="btn" onclick={() => (selecting = true)}>Select</button>
        <button class="btn" onclick={openFolder}>Open folder</button>
      {/if}
    </div>
    <div class="sg">
      {#if shots.data}
        {#each shown as s (s.id)}
          <a href={href({ name: 'instance', id: instanceId, tab: 'screenshots', sub: s.id })} class="sh" class:on={picked.includes(s.id)} style="background: {s.art.background}" aria-label="{selecting ? (picked.includes(s.id) ? 'Unselect' : 'Select') : 'Open'} {when(s.takenAt)}" onclick={(e) => toggle(e, s.id)}><span class="cap tnum">{when(s.takenAt)}</span></a>
        {:else}
          <Empty text={emptyText} style="grid-column: 1 / -1" />
        {/each}
      {:else}
        {#each { length: 12 } as _, k (k)}<div class="sk" style="aspect-ratio: 16/9; border-radius: 8px" aria-hidden="true"></div>{/each}
      {/if}
    </div>
  </div>
</div>

<style>
  .sg{display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:12px}
  .sh{position:relative;aspect-ratio:16/9;border-radius:8px;overflow:hidden;border:0;padding:0;display:block;width:100%}
  .sh .cap{position:absolute;inset:auto 0 0 0;padding:20px 10px 8px;background:linear-gradient(transparent,rgba(0,0,0,.7));color:#fff;font-size:12px;text-align:left;opacity:0;transition:opacity 140ms ease}
  .sh:hover .cap,.sh:focus-visible .cap{opacity:1}
  /* A picked tile wears the filmstrip's selected border. */
  .sh.on{box-shadow:inset 0 0 0 2px var(--text)}
</style>

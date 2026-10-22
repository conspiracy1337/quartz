<script lang="ts">
  import { api } from '../api';
  import { contentArt } from '../api/content';
  import type { SnapshotRow, SnapshotSummary } from '../api/types';
  import { plural, since } from '../format';
  import { rolledBack } from '../instanceActions';
  import { PROJECT_TYPES } from '../labels';
  import { query } from '../query.svelte';
  import { session } from '../session.svelte';
  import Check from '../ui/Check.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Empty from '../ui/Empty.svelte';
  import Icon from '../ui/Icon.svelte';
  import Menu, { below } from '../ui/Menu.svelte';
  import MenuConfirm from '../ui/MenuConfirm.svelte';
  import Setting from '../ui/Setting.svelte';
  import Tile from '../ui/Tile.svelte';
  import Toggle from '../ui/Toggle.svelte';

  /**
   * Roll back to a snapshot (composed: no board). The snapshots list is the
   * Trash list's rows, the changes are the update review's rows with their
   * select boxes, Delete asks in Storage's confirm menu. The changes the
   * snapshot's update touched start ticked; untick any to keep them.
   */
  interface Props { instanceId: string; snapshot?: string | undefined }
  let { instanceId, snapshot }: Props = $props();

  const inst = query(() => api.instances.get(instanceId));
  // svelte-ignore state_referenced_locally
  const list = query(() => api.snapshots.list(instanceId), `snapshots.${instanceId}`);
  // svelte-ignore state_referenced_locally
  let chosen = $state(snapshot ?? '');
  const cur = $derived(list.data?.find((s) => s.id === chosen) ?? list.data?.[0]);
  const diff = query(() => (cur ? api.snapshots.diff(instanceId, cur.id) : Promise.resolve(null)));
  let picked = $state<Record<string, boolean>>({});
  let busy = $state(false);
  let ask = $state<{ x: number; y: number } | null>(null);

  // A snapshot's own changes start ticked whenever another one is picked.
  let pickedFor = '';
  $effect(() => {
    const d = diff.data;
    if (!d || d.snapshot.id === pickedFor) return;
    pickedFor = d.snapshot.id;
    picked = Object.fromEntries(d.rows.map((r) => [r.key, r.default && !r.unavailable]));
  });
  const rows = $derived(diff.data?.snapshot.id === cur?.id ? (diff.data?.rows ?? []) : []);
  const keys = $derived(rows.filter((r) => picked[r.key] && !r.unavailable).map((r) => r.key));

  const title = (s: SnapshotSummary) => (s.title ? `Before ${s.title}` : 'Snapshot you made');
  const left = (s: SnapshotSummary) => `Expires in ${plural(Math.max(1, Math.ceil(((s.expires ?? 0) - Date.now()) / 86_400_000)), 'day')}`;
  const NOUN: Record<string, string> = Object.fromEntries(PROJECT_TYPES.flatMap((t) => (t.content ? [[t.content, t.noun]] : [])));
  const CHANGE: Record<SnapshotRow['change'], string> = { changed: '', enable: 'turns back on', disable: 'turns back off', restore: 'comes back', remove: 'added since, will be removed', set: '' };
  function note(r: SnapshotRow) {
    if (r.unavailable) return r.unavailable;
    // Instance-wide rows have long values: they go on the second line, where there is room.
    if (r.change === 'set') return r.from || r.to ? `${r.from || 'None'} to ${r.to || 'None'}` : (r.detail ?? '');
    const what = NOUN[r.kind];
    const parts = [what ? what.charAt(0).toUpperCase() + what.slice(1) : '', CHANGE[r.change], r.detail ?? ''].filter(Boolean);
    return parts.join(', ');
  }
  const FOLDERS: Record<string, string> = { saves: 'data packs', resourcepacks: 'resource packs', shaderpacks: 'shader packs', config: 'config files', defaultconfigs: 'default configs' };
  const shared = $derived.by(() => {
    const names = (diff.data?.shared ?? []).map((f) => FOLDERS[f] ?? f);
    if (!names.length) return '';
    const list = names.length === 1 ? names[0]! : `${names.slice(0, -1).join(', ')} and ${names.at(-1)}`;
    return `${list.charAt(0).toUpperCase()}${list.slice(1)} are shared with other instances, so they are left out.`;
  });

  async function create() {
    try {
      const s = await api.snapshots.create(instanceId);
      chosen = s.id;
    } catch (e) {
      session.fail('Create snapshot', e);
    }
  }

  async function keep(on: boolean) {
    if (!cur) return;
    try {
      await api.snapshots.setKeep(instanceId, cur.id, on);
    } catch (e) {
      session.fail('Keep snapshot', e);
    }
  }

  async function remove() {
    ask = null;
    if (!cur) return;
    const id = cur.id;
    try {
      await api.snapshots.remove(instanceId, id);
      chosen = '';
    } catch (e) {
      session.fail('Delete snapshot', e);
    }
  }

  async function rollBack() {
    if (!cur || busy || keys.length === 0) return;
    busy = true;
    try {
      const r = await api.snapshots.rollback(instanceId, cur.id, keys);
      session.close();
      rolledBack(instanceId, r);
    } catch (e) {
      busy = false;
      session.fail('Roll back', e);
    }
  }
</script>

<Dialog title="Roll back" sub="{inst.data?.name ?? ''}: put back how it was before a change." width={880} onclose={() => session.close()}>
  <div class="rb">
    <div class="field" style="min-height: 0">
      <span class="field-label rb-label">Snapshots<button class="btn btn-sm btn-ghost" onclick={create}><Icon name="plus" size={12} sw={2} />Create snapshot</button></span>
      <div class="list rb-list">
        {#each list.data ?? [] as s (s.id)}
          <button class="row snap" class:sel={s.id === cur?.id} onclick={() => (chosen = s.id)}>
            <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{title(s)}</span><span class="trunc small faint" style="display: block">{since(s.at)}</span></span>
            {#if s.keep}<span class="badge badge-info">Kept</span>{:else}<span class="small muted tnum">{left(s)}</span>{/if}
          </button>
        {:else}
          {#if list.data}<Empty text="No snapshots yet. One is taken before every update." />
          {:else}{#each { length: 3 } as _, k (k)}<div class="row snap" aria-hidden="true"><div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: {[70, 56, 64][k]}%"></div><div class="sk" style="height: 10px; width: 36%"></div></div><div class="sk" style="height: 10px; width: 64px"></div></div>{/each}{/if}
        {/each}
      </div>
    </div>
    <div class="field" style="min-height: 0">
      <span class="field-label rb-label">Changes to roll back{#if cur}<button class="btn btn-sm btn-ghost" onclick={(e) => (ask = below(e))}>Delete snapshot</button>{/if}</span>
      <div class="list rb-list">
        {#each rows as r (r.key)}
          {@const art = contentArt(r.name, r.icon)}
          <div class="row ch">
            {#if r.unavailable}<Check on={false} />{:else}<Check on={!!picked[r.key]} label="Roll back {r.name}" onchange={(v) => (picked[r.key] = v)} />{/if}
            <Tile {art} />
            <span style="min-width: 0" class:wide={r.change === 'set'}><span class="trunc" style="display: block; font-weight: 500">{r.name}</span><span class="trunc small faint" style="display: block" class:warn={!!r.unavailable}>{note(r)}</span></span>
            {#if r.change !== 'set'}<span class="mono muted small ch-v"><span class="trunc">{r.from || 'None'}</span><Icon name="arrow-right" size={12} sw={2} /><span class="trunc" style="color: var(--text)">{r.to || 'None'}</span></span>{/if}
          </div>
        {:else}
          {#if diff.data || (list.data && !cur)}<Empty text={cur ? 'Nothing changed since this snapshot.' : 'Pick a snapshot to see what it puts back.'} />
          {:else}{#each { length: 5 } as _, k (k)}<div class="row ch" aria-hidden="true"><div class="sk" style="width: 16px; height: 16px; border-radius: 4px"></div><div class="sk" style="width: 32px; height: 32px; border-radius: 7px"></div><div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: {[40, 30, 46, 34, 52][k]}%"></div><div class="sk" style="height: 10px; width: 24%"></div></div><div class="sk" style="height: 10px; width: 80%"></div></div>{/each}{/if}
        {/each}
      </div>
    </div>
  </div>
  {#if cur}
    <div class="card"><Setting title="Keep this snapshot" desc={cur.keep ? 'Kept until you delete it.' : 'Snapshots are deleted 7 days after they were taken.'}><Toggle on={cur.keep} label="Keep this snapshot" onchange={keep} /></Setting></div>
  {/if}
  <span class="small faint">Worlds are never rolled back.{shared ? ` ${shared}` : ''}{diff.data?.partial?.length ? ` Some files in ${diff.data.partial.join(', ')} were too big to keep.` : ''}{diff.data && !diff.data.undoable ? ' Snapshots are off, so this rollback can’t be undone.' : ''}</span>
  {#snippet foot()}
    <span class="small faint tnum" style="margin-right: auto">{plural(keys.length, 'change')} selected</span>
    <button class="btn" onclick={() => session.close()}>Cancel</button>
    <button class="btn btn-accent" disabled={busy || keys.length === 0} onclick={rollBack}>Roll back {keys.length}</button>
  {/snippet}
</Dialog>
{#if ask && cur}
  {@const a = ask}
  <Menu x={a.x} y={a.y} label="Delete snapshot" onclose={() => (ask = null)}>
    <MenuConfirm text="Delete this snapshot? Its old files are freed and it can't be rolled back to." action="Delete" oncancel={() => (ask = null)} onconfirm={remove} />
  </Menu>
{/if}

<style>
  .rb{display:grid;grid-template-columns:300px minmax(0,1fr);gap:16px;height:360px}
  .rb-label{display:flex;align-items:center;justify-content:space-between;height:28px}
  .rb-list{flex:1 1 auto;min-height:0;overflow-y:auto;overscroll-behavior:contain}
  .snap{grid-template-columns:minmax(0,1fr) auto;height:56px}
  button.snap{border:0;border-top:1px solid var(--line);background:transparent;width:100%;text-align:left;color:var(--text)}
  button.snap:first-child{border-top:0}
  button.snap.sel{background:var(--raise)}
  .ch{grid-template-columns:16px 32px minmax(0,1fr) 180px;height:56px}
  .ch-v{display:grid;grid-template-columns:minmax(0,1fr) 12px minmax(0,1fr);gap:6px;align-items:center}
  .warn{color:var(--warn)}
  .wide{grid-column:3 / -1}
</style>

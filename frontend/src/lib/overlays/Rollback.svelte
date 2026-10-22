<script lang="ts">
  import { api } from '../api';
  import { contentArt } from '../api/content';
  import type { RestorePoint, SnapshotRow } from '../api/types';
  import { plural, since } from '../format';
  import { rolledBack } from '../instanceActions';
  import { PROJECT_TYPES } from '../labels';
  import { query } from '../query.svelte';
  import { changeLabel, pointLeft, pointTitle } from '../restore';
  import { session } from '../session.svelte';
  import Check from '../ui/Check.svelte';
  import ConfirmMenu, { Confirm } from '../ui/ConfirmMenu.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Empty from '../ui/Empty.svelte';
  import Icon from '../ui/Icon.svelte';
  import Setting from '../ui/Setting.svelte';
  import Tile from '../ui/Tile.svelte';
  import Toggle from '../ui/Toggle.svelte';

  /**
   * Roll back to a restore point (DlgRollback.dc.html, board A): the points on a timeline,
   * each with the changes made after it, and the picked point's changes with their select
   * boxes, all ticked (untick any to keep it as it is now). + makes a point; the bin
   * deletes the picked one after asking in a menu.
   */
  interface Props { instanceId: string; snapshot?: string | undefined }
  let { instanceId, snapshot }: Props = $props();

  const inst = query(() => api.instances.get(instanceId));
  // svelte-ignore state_referenced_locally
  const list = query(() => api.snapshots.list(instanceId), `snapshots.${instanceId}`);
  // The point the game crashed after, marked on the timeline.
  const suspect = query(() => api.snapshots.suspect(instanceId));
  // svelte-ignore state_referenced_locally
  let chosen = $state(snapshot ?? '');
  const cur = $derived(list.data?.find((p) => p.id === chosen) ?? list.data?.[0]);
  const diff = query(() => (cur ? api.snapshots.diff(instanceId, cur.id) : Promise.resolve(null)));
  let picked = $state<Record<string, boolean>>({});
  let busy = $state(false);
  const confirm = new Confirm();

  // Every change starts ticked whenever another point is picked.
  let pickedFor = '';
  $effect(() => {
    const d = diff.data;
    if (!d || d.point.id === pickedFor) return;
    pickedFor = d.point.id;
    picked = Object.fromEntries(d.rows.map((r) => [r.key, r.default && !r.unavailable]));
  });
  const rows = $derived(diff.data?.point.id === cur?.id ? (diff.data?.rows ?? []) : []);
  const keys = $derived(rows.filter((r) => picked[r.key] && !r.unavailable).map((r) => r.key));

  // A point lists at most this many of its changes; the rest are counted.
  const SHOWN = 3;
  const log = (p: RestorePoint) => (p.manual ? [since(p.at)] : p.changes.slice(0, SHOWN).map((c) => changeLabel(c.title)));
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
      const p = await api.snapshots.create(instanceId);
      chosen = p.id;
    } catch (e) {
      session.fail('Create restore point', e);
    }
  }

  async function keep(on: boolean) {
    if (!cur) return;
    try {
      await api.snapshots.setKeep(instanceId, cur.id, on);
    } catch (e) {
      session.fail('Keep restore point', e);
    }
  }

  async function remove(id: string) {
    try {
      await api.snapshots.remove(instanceId, id);
      chosen = '';
    } catch (e) {
      session.fail('Delete restore point', e);
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

<Dialog title="Roll back" sub="{inst.data?.name ?? ''}: go back to how it was before a set of changes." width={880} onclose={() => session.close()}>
  <div class="rb">
    <div class="field" style="min-height: 0">
      <span class="field-label rb-label">Restore points<span style="display: flex; gap: 2px">
        <button class="btn btn-sm btn-ghost btn-icon" aria-label="Create restore point" title="Create restore point" onclick={create}><Icon name="plus" size={12} sw={2} /></button>
        {#if cur}{@const id = cur.id}<button class="btn btn-sm btn-ghost btn-icon" aria-label="Delete restore point" title="Delete restore point" onclick={(e) => confirm.ask(e, "Delete this restore point? Its old files are freed and it can't be rolled back to.", 'Delete', () => void remove(id))}><Icon name="trash" size={12} sw={1.8} /></button>{/if}
      </span></span>
      <div class="list rb-list">
        {#each list.data ?? [] as p (p.id)}
          {@const lines = log(p)}
          <button class="tl" class:sel={p.id === cur?.id} onclick={() => (chosen = p.id)}>
            <span class="rail"><span class="node"></span></span>
            <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{pointTitle(p)}</span>
              <span class="log">{#each lines as l, k (k)}<span class="trunc">{l}</span>{/each}{#if p.changes.length > SHOWN}<span>and {plural(p.changes.length - SHOWN, 'more change')}</span>{/if}</span></span>
            {#if suspect.data?.id === p.id}<span class="badge badge-danger">Crashed</span>
            {:else if p.keep}<span class="badge badge-info">Kept</span>
            {:else if p.expires}<span class="small muted tnum">{pointLeft(p)}</span>
            {:else}<span class="small faint tnum">{since(p.changes.at(-1)?.at ?? p.at)}</span>{/if}
          </button>
        {:else}
          {#if list.data}<Empty text="No restore points yet. The first change after you play makes one." />
          {:else}{#each { length: 3 } as _, k (k)}<div class="tl" aria-hidden="true"><span class="rail"><span class="node"></span></span><div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: {[70, 56, 64][k]}%"></div><div class="sk" style="height: 10px; width: 46%"></div></div><div class="sk" style="height: 10px; width: 40px"></div></div>{/each}{/if}
        {/each}
      </div>
    </div>
    <div class="field" style="min-height: 0">
      <span class="field-label rb-label">Changes to roll back</span>
      <div class="list rb-list">
        {#each rows as r (r.key)}
          {@const art = contentArt(r.name, r.icon)}
          <div class="row ch">
            {#if r.unavailable}<Check on={false} />{:else}<Check on={!!picked[r.key]} label="Roll back {r.name}" onchange={(v) => (picked[r.key] = v)} />{/if}
            <span class:off={!!r.unavailable}><Tile {art} /></span>
            <span style="min-width: 0" class:wide={r.change === 'set'}><span class="trunc" class:off={!!r.unavailable} style="display: block; font-weight: 500">{r.name}</span><span class="trunc small faint" style="display: block" class:warn={!!r.unavailable}>{note(r)}</span></span>
            {#if r.change !== 'set'}<span class="mono muted small ch-v" class:off={!!r.unavailable}><span class="trunc">{r.from || 'None'}</span><Icon name="arrow-right" size={12} sw={2} /><span class="trunc" style="color: var(--text)">{r.to || 'None'}</span></span>{/if}
          </div>
        {:else}
          {#if diff.data || (list.data && !cur)}<Empty text={cur ? 'Nothing changed since this restore point.' : 'Pick a restore point to see what it puts back.'} />
          {:else}{#each { length: 5 } as _, k (k)}<div class="row ch" aria-hidden="true"><div class="sk" style="width: 16px; height: 16px; border-radius: 4px"></div><div class="sk" style="width: 32px; height: 32px; border-radius: 7px"></div><div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: {[40, 30, 46, 34, 52][k]}%"></div><div class="sk" style="height: 10px; width: 24%"></div></div><div class="sk" style="height: 10px; width: 80%"></div></div>{/each}{/if}
        {/each}
      </div>
    </div>
  </div>
  {#if cur}
    <div class="card"><Setting title="Keep this restore point" desc={cur.keep ? 'Kept until you delete it.' : 'Restore points are deleted 7 days after you play past them. The newest one always stays.'}><Toggle on={cur.keep} label="Keep this restore point" onchange={keep} /></Setting></div>
  {/if}
  <span class="small faint">Worlds are never rolled back.{shared ? ` ${shared}` : ''}{diff.data?.partial?.length ? ` Some files in ${diff.data.partial.join(', ')} were too big to keep.` : ''}{diff.data && !diff.data.undoable ? ' Restore points are off, so this rollback can’t be undone.' : ' Rolling back can be undone from the next restore point.'}</span>
  {#snippet foot()}
    <span class="small faint tnum" style="margin-right: auto">{plural(keys.length, 'change')} selected</span>
    <button class="btn" onclick={() => session.close()}>Cancel</button>
    <button class="btn btn-accent" disabled={busy || keys.length === 0} onclick={rollBack}>Roll back {keys.length}</button>
  {/snippet}
</Dialog>
<ConfirmMenu {confirm} />

<style>
  .rb{display:grid;grid-template-columns:300px minmax(0,1fr);gap:16px;height:392px}
  .rb-label{display:flex;align-items:center;justify-content:space-between;height:28px}
  .rb-list{flex:1 1 auto;min-height:0;overflow-y:auto;overscroll-behavior:contain}
  /* The timeline: a rail with a node per point, the picked one filled. */
  .tl{display:grid;grid-template-columns:16px minmax(0,1fr) auto;column-gap:12px;align-items:start;padding:12px 14px 12px 12px;border:0;background:transparent;width:100%;text-align:left;color:var(--text);position:relative}
  button.tl:hover{background:color-mix(in srgb,var(--raise) 60%,transparent)}
  .tl.sel{background:var(--raise)}
  .rail{position:relative;display:flex;justify-content:center;align-self:stretch}
  .rail::before{content:"";position:absolute;top:-12px;bottom:-12px;width:1px;background:var(--line-2)}
  .tl:first-child .rail::before{top:8px}
  .tl:last-child .rail::before{bottom:calc(100% - 8px)}
  .node{position:relative;margin-top:4px;width:10px;height:10px;border-radius:50%;background:var(--panel);border:2px solid var(--faint)}
  .tl.sel .node{border-color:var(--accent);background:var(--accent)}
  .log{display:flex;flex-direction:column;margin-top:4px}
  .log span{font-size:12px;line-height:18px;color:var(--faint)}
  .ch{grid-template-columns:16px 32px minmax(0,1fr) 180px;height:52px}
  .ch-v{display:grid;grid-template-columns:minmax(0,1fr) 12px minmax(0,1fr);gap:6px;align-items:center}
  .warn{color:var(--warn)}
  .off{opacity:.45}
  .wide{grid-column:3 / -1}
</style>

<script lang="ts">
  import { api } from '$lib/api';
  import type { ActivityKind, Instance, SyncEntry } from '$lib/api/types';
  import { Day, fullDate, sentenceList } from '$lib/format';
  import { loaderLabel } from '$lib/labels';
  import { query } from '$lib/query.svelte';
  import { href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Seg from '$lib/ui/Seg.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Tile from '$lib/ui/Tile.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';
  import Empty from '$lib/ui/Empty.svelte';

  interface Props { instance: Instance }
  let { instance: i }: Props = $props();

  const activity = query(() => api.instances.activity(i.id));
  const updates = query(() => api.instances.pendingUpdates(i.id));
  let filter = $state<'all' | ActivityKind>('all');
  // The instance's own Discord switch (Integrations holds the global one).
  async function setDiscord(on: boolean) {
    try {
      await api.instances.setDiscord(i.id, on);
    } catch (e) {
      session.fail('Discord status', e);
    }
  }
  const FILTERS = [{ value: 'all', label: 'All' }, { value: 'session', label: 'Sessions' }, { value: 'change', label: 'Changes' }] as const;
  const TONES = { muted: 'var(--muted)', accent: 'var(--accent)', danger: 'var(--danger)' } as const;
  const events = $derived((activity.data ?? []).filter((e) => filter === 'all' || e.kind === filter));
  const gb = (mb: number) => Math.round(mb / 1024);
  const contentLine = $derived(`${i.counts.mods} mods, ${i.counts.resourcepacks} packs, ${i.counts.worlds} worlds`);
  // What this instance shares, by the Syncing page's names (the shared keybinds first).
  const sync = query(() => api.sync.groups());
  const syncText = $derived.by(() => {
    const g = sync.data;
    if (!g) return '';
    // Built-in targets read as words ("server list"); mod names and custom paths keep their case.
    const joined = (l: SyncEntry[], lower: boolean) => l.filter((t) => t.enabled && i.sync.join.includes(t.id)).map((t) => (lower ? t.name.toLowerCase() : t.name || t.path));
    const names = [...joined(g.files, true), ...joined(g.folders, true), ...joined(g.mods, false), ...joined(g.custom, false)];
    if (i.sync.keys) names.unshift('keybinds');
    return names.length ? `${sentenceList(names)}.` : 'Nothing shared.';
  });
</script>

<div class="ov-wrap">
  <div class="ov">
    <section class="card ov-card ov-act" aria-label="Activity" style="grid-area: activity">
      <div class="card-head"><h2 class="h3">Activity</h2><Seg options={FILTERS} bind:value={filter} label="Activity filter" /></div>
      <div class="ov-body">
        {#each events as e, k (k)}
          <div class="ev">
            <span class="ev-ic" style="color: {TONES[e.tone]}" aria-hidden="true"><Icon name={e.icon} size={14} sw={1.8} outline /></span>
            <span style="min-width: 0"><span class="trunc" style="display: block">{e.title}</span><span class="trunc small faint" style="display: block">{e.detail}</span></span>
            <span class="small faint tnum" style="white-space: nowrap">{Day(e.at)}</span>
          </div>
        {:else}
          {#if activity.data}<Empty text="No activity yet. Sessions and changes show up here." />
          {:else}{#each { length: 5 } as _, k (k)}<div class="ev" aria-hidden="true"><div class="sk" style="width: 28px; height: 28px; border-radius: 7px"></div><div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: {[48, 36, 56, 40, 30][k]}%"></div><div class="sk" style="height: 10px; width: 28%"></div></div><div class="sk" style="height: 10px; width: 48px"></div></div>{/each}{/if}
        {/each}
      </div>
    </section>
    <section class="card ov-card" aria-label="Details" style="grid-area: details">
      <div class="card-head"><h2 class="h3">Details</h2><button class="btn btn-sm btn-ghost" onclick={() => api.instances.openFolder(i.id)}>Open folder</button></div>
      <div class="ov-body">
        <div class="kv ov-kv">
          <span>Minecraft</span><span>{i.minecraft}</span>
          <span>Loader</span><span>{i.loader ? loaderLabel(i.loader) : 'None'}</span>
          <span>Java</span><span>{i.java}</span>
          <span>Memory</span><span>{gb(i.memoryMB.min)} to {gb(i.memoryMB.max)} GB</span>
          <span>Content</span><span class="tnum">{contentLine}</span>
          <span>Created</span><span>{fullDate(i.created)}</span>
          <span>Folder</span><span class="trunc mono" style="color: var(--muted)">{i.folder}</span>
        </div>
      </div>
    </section>
    <section class="card ov-card" aria-label="Updates" style="grid-area: updates">
      <div class="card-head"><h2 class="h3">Updates <span class="count">{updates.data?.length ?? ''}</span></h2><a href={href({ name: 'instance', id: i.id, tab: 'mods', sub: 'updates' })} class="btn btn-sm btn-accent">Review</a></div>
      <div class="ov-body">
        {#each updates.data ?? [] as u (u.name)}
          <div class="up"><Tile art={u.art} size="sm" /><span class="trunc">{u.name}</span><span class="mono muted">{u.from} to {u.to}</span></div>
        {:else}
          {#if updates.data}<Empty text="Everything is up to date." />
          {:else}{#each { length: 3 } as _, k (k)}<div class="up" aria-hidden="true"><div class="sk" style="width: 24px; height: 24px; border-radius: 6px"></div><div class="sk" style="height: 12px; width: {[44, 32, 52][k]}%"></div><div class="sk" style="height: 10px; width: 90px"></div></div>{/each}{/if}
        {/each}
      </div>
    </section>
    <section class="card ov-card ov-fixed" aria-label="Protection and sync" style="grid-area: protect">
      <div class="card-head"><h2 class="h3">Protection and sync</h2></div>
      <div class="ov-body">
        <Setting title="Sandbox" desc={i.sandboxed ? "Mods only reach this instance's folder." : 'Off. Mods can reach your files.'} oneLine href={href({ name: 'instance', id: i.id, tab: 'settings', sub: 'sandbox' })}><span class="badge badge-info">{i.sandboxed ? 'On' : 'Off'}</span></Setting>
        <div class="divider"></div>
        <Setting title="Syncing" desc={syncText} oneLine href={href({ name: 'syncing' })}><span class="badge">{i.syncTargets} targets</span></Setting>
        <div class="divider"></div>
        <Setting title="Discord status" desc="Show this instance while you play." oneLine><Toggle on={i.discordStatus} label="Discord status" onchange={setDiscord} /></Setting>
      </div>
    </section>
  </div>
</div>

<style>
  /* A 2x2 grid that fills the space under the header. Row 1 takes what is left, row 2 has one fixed height,
     so every card edge lines up whatever the data. Cards keep their header and scroll their own body. */
  .ov-wrap{flex:1 1 auto;min-height:0;overflow:auto}
  .ov{height:100%;min-height:440px;display:grid;gap:var(--gap);padding:20px 28px 28px;
    grid-template-columns:minmax(0,1.4fr) minmax(0,1fr);grid-template-rows:minmax(0,1fr) 224px;
    grid-template-areas:"activity details" "updates protect"}
  .ov-card{display:flex;flex-direction:column;min-height:0;min-width:0;overflow:hidden}
  .ov-card > .card-head{flex:0 0 44px}
  .ov-body{flex:1 1 auto;min-height:0;overflow-y:auto;overscroll-behavior:contain}
  .ov-body > :first-child{border-top:0}
  /* Fixed-content cards: titles and descriptions stay on one line, so the content always fits and never scrolls. */
  .ov-fixed .ov-body{overflow:hidden}
  .ov-fixed :global(.setting > span:first-child){min-width:0}
  @media (max-width:1100px){
    .ov{height:auto;grid-template-columns:minmax(0,1fr);grid-template-rows:none;grid-template-areas:"activity" "updates" "details" "protect"}
    .ov-card{height:auto}
    .ov-act{height:360px}
  }
  .ev{display:grid;grid-template-columns:28px minmax(0,1fr) auto;gap:12px;align-items:center;min-height:52px;padding:8px var(--inset);border-top:1px solid var(--line)}
  .up{display:grid;grid-template-columns:20px minmax(0,1fr) auto;column-gap:12px;align-items:center;height:48px;padding:0 var(--inset);border-top:1px solid var(--line)}
  .ov-kv{padding:14px var(--inset);grid-template-columns:96px minmax(0,1fr);row-gap:10px}
  .ev-ic{width:28px;height:28px;border-radius:7px;display:grid;place-items:center;background:var(--raise)}
</style>

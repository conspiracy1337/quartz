<script lang="ts">
  import { api } from '$lib/api';
  import type { DiffKind, Instance } from '$lib/api/types';
  import { errorText, plural } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import { go, href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import { tasks } from '$lib/tasks.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';

  interface Props { instance: Instance }
  let { instance: i }: Props = $props();

  const diff = query(() => api.modpacks.diff(i.id));
  let tab = $state<'All changes' | DiffKind>('All changes');
  let backup = $state(true);
  const MARK: Record<DiffKind, { sym: string; color: string }> = {
    Added: { sym: '+', color: 'var(--accent)' },
    Updated: { sym: '~', color: 'var(--info)' },
    Removed: { sym: '-', color: 'var(--danger)' },
  };
  const TABS = ['All changes', 'Added', 'Updated', 'Removed'] as const;
  const rows = $derived((diff.data?.rows ?? []).filter((r) => tab === 'All changes' || r.kind === tab));
  const countOf = (t: (typeof TABS)[number]) => (diff.data?.rows ?? []).filter((r) => t === 'All changes' || r.kind === t).length;
  const colorOf = (t: (typeof TABS)[number]) => (t === 'All changes' ? 'var(--text)' : MARK[t].color);

  let busy = $state(false);

  async function update() {
    busy = true;
    try {
      const job = await api.modpacks.update(i.id, backup);
      if (job) await tasks.load();
      go({ name: 'downloads' });
    } catch (e) {
      busy = false;
      session.notify({ tone: 'error', subject: `Couldn't update ${i.name}`, text: errorText(e) });
    }
  }
</script>

<div class="topbar">
  <a href={href({ name: 'instance', id: i.id, tab: 'overview' })} class="btn btn-sm btn-ghost btn-icon" aria-label="Back"><Icon name="chevron-left" size={14} sw={2} /></a>
  <h1 class="h2 trunc">{i.name}</h1>
  {#if diff.data}<span class="mono muted" style="display: flex; gap: 8px; align-items: center; white-space: nowrap">{diff.data.from}<Icon name="arrow-right" size={12} sw={2} /><span style="color: var(--text)">{diff.data.to}</span></span>{/if}
</div>
<div class="scroll">
  {#if diff.error}
    <div class="page" style="padding-top: 16px"><p class="muted" style="margin: 0">{errorText(diff.error)}</p></div>
  {:else if diff.data}
    {@const d = diff.data}
    <div class="page rb" style="padding-top: 16px">
      <div style="display: flex; flex-direction: column; gap: 14px; min-width: 0">
        <div class="sum" role="tablist">
          {#each TABS as t (t)}<button class:on={tab === t} onclick={() => (tab = t)} role="tab" aria-selected={tab === t}><span class="small faint" style="display: block">{t}</span><span class="tnum" style="font-size: 18px; line-height: 24px; font-weight: 600; color: {colorOf(t)}">{countOf(t)}</span></button>{/each}
        </div>
        <div class="list">
          {#each rows as r, ri (ri)}
            <div class="row dr"><span class="mk" style="background: color-mix(in srgb, {MARK[r.kind].color} 16%, transparent); color: {MARK[r.kind].color}" aria-label={r.kind}>{MARK[r.kind].sym}</span><span class="trunc">{r.name}</span><span class="mono small muted">{r.version}</span></div>
          {/each}
        </div>
      </div>
      <aside style="display: flex; flex-direction: column; gap: 14px">
        <section class="card card-pad" style="display: flex; flex-direction: column; gap: 8px" aria-label="Release notes">
          <h2 class="h3">What changed</h2>
          <p class="small muted" style="margin: 0">{d.summary || 'The pack has no release notes for this version.'}{#if d.game} It moves to {d.game}.{/if}</p>
        </section>
        <section class="card" aria-label="Your changes">
          <div class="card-head"><h2 class="h3">Your edits</h2><span class="badge badge-accent">Kept</span></div>
          <div class="card-pad small muted" style="display: flex; flex-direction: column; gap: 6px">
            <span>{d.keptEdits.length ? `${plural(d.keptEdits.length, 'config file')} you changed will not be overwritten:` : 'The update changes none of the files you edited.'}</span>
            {#each d.keptEdits as f (f)}<span class="mono trunc" style="color: var(--text)">{f}</span>{/each}
          </div>
        </section>
        <section class="card" aria-label="Before updating">
          <div class="setting" style="min-height: 48px"><span class="setting-title">Back up worlds first</span><Toggle bind:on={backup} label="Back up worlds first" /></div>
        </section>
        <button class="btn btn-accent btn-block" disabled={busy || session.running[i.id]} onclick={update}>Update to {d.to}</button>
      </aside>
    </div>
  {/if}
</div>

<style>
  .rb{display:grid;grid-template-columns:minmax(0,1fr) 300px;gap:16px;align-items:start}
  @media (max-width:1100px){.rb{grid-template-columns:minmax(0,1fr)}}
  .dr{grid-template-columns:20px minmax(0,1fr) auto;height:38px}
  .sum{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:1px;background:var(--line);border:1px solid var(--line);border-radius:10px;overflow:hidden}
  .sum > button{background:var(--panel);padding:10px 14px;border:0;text-align:left;color:var(--text)}
  .sum > button.on{background:var(--raise)}
  .mk{width:20px;height:20px;border-radius:4px;display:grid;place-items:center;font-family:var(--mono);font-size:12px;font-weight:600}
</style>

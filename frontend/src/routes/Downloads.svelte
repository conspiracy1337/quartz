<script lang="ts">
  import { api } from '$lib/api';
  import { bytes, percent, plural } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import { session } from '$lib/session.svelte';
  import Bar from '$lib/ui/Bar.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import PageHeader from '$lib/ui/PageHeader.svelte';
  import Tabs from '$lib/ui/Tabs.svelte';
  import Tile from '$lib/ui/Tile.svelte';

  const active = query(() => api.downloads.active());
  const done = query(() => api.downloads.finished());
  let tab = $state<0 | 1 | 2>(0);
  let open = $state<Record<string, boolean>>({ d1: true });
  let paused = $state<Record<string, boolean>>({});

  const list = $derived(active.data ?? []);
  const finished = $derived((done.data ?? []).filter((d) => d.ok));
  const failed = $derived((done.data ?? []).filter((d) => !d.ok));
  const allPaused = $derived(list.length > 0 && list.every((d) => paused[d.id]));
  const sumDone = $derived(list.reduce((n, d) => n + d.doneBytes, 0));
  const sumTotal = $derived(list.reduce((n, d) => n + d.totalBytes, 0));
  const sumPct = $derived(percent(sumDone, sumTotal));
  const shown = $derived(tab === 1 ? finished : failed);
  const shownDone = $derived(shown.reduce((n, d) => n + d.doneBytes, 0));
  const shownTotal = $derived(shown.reduce((n, d) => n + d.totalBytes, 0));
  const isFinished = $derived(tab === 1);
  const mb = (n: number) => `${Math.round(n / 1e5) / 10}`;
  const tabItems = $derived([
    { key: 0 as const, label: 'Active', count: list.length },
    { key: 1 as const, label: 'Finished', count: finished.length },
    { key: 2 as const, label: 'Failed', count: failed.length },
  ]);
  const eta = (s?: number) => (s === undefined ? '' : s < 60 ? `${s} s left` : `${Math.round(s / 60)} min left`);

  function toggleAll() {
    paused = allPaused ? {} : Object.fromEntries(list.map((d) => [d.id, true]));
  }
</script>

<PageHeader title="Downloads">
  {#snippet tabs()}<Tabs items={tabItems} active={tab} label="Downloads" onselect={(k) => (tab = k)} />{/snippet}
</PageHeader>
<div class="scroll">
  <div class="page" style="gap: 12px">
    {#if tab === 0}
      <div class="list" role="list" aria-label="Active downloads">
        <div class="dl-row dl-sum" role="listitem" aria-label="All downloads">
          <span style="min-width: 0; grid-column: 1 / span 3"><span class="trunc" style="display: block; font-weight: 600">All downloads</span><span class="trunc small faint tnum" style="display: block">{plural(list.length, 'download')}, {allPaused ? 'paused' : '12.4 MB/s'}</span></span>
          <span class="dl-eta" style="color: {allPaused ? 'var(--warn)' : 'var(--faint)'}">{allPaused ? 'Paused' : '1 min left'}</span>
          <span class="dl-detail muted">{mb(sumDone)} of {mb(sumTotal)} MB</span>
          <span class="dl-pct">{sumPct}%</span>
          <Bar pct={sumPct} live={!allPaused} label="All downloads" />
          <span class="dl-acts">
            <button class="btn btn-icon btn-sm btn-ghost" onclick={toggleAll} aria-label={allPaused ? 'Resume all downloads' : 'Pause all downloads'}><Icon name={allPaused ? 'play' : 'pause'} size={12} /></button>
            <button class="btn btn-icon btn-sm btn-ghost" aria-label="Cancel all downloads"><Icon name="x" size={12} sw={2} /></button>
          </span>
        </div>
        {#each list as d (d.id)}
          {@const isPaused = !!paused[d.id]}
          {@const pct = percent(d.doneBytes, d.totalBytes)}
          <div role="listitem">
            <div class="dl-row">
              <button class="dl-chev" class:open={open[d.id]} onclick={() => (open[d.id] = !open[d.id])} aria-expanded={!!open[d.id]} aria-label="Show steps for {d.name}"><Icon name="chevron-right" size={12} sw={2.2} /></button>
              <Tile art={d.art} />
              <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{d.name}</span><span class="trunc small faint" style="display: block">{d.sub}</span></span>
              <span class="dl-eta" style="color: {isPaused ? 'var(--warn)' : 'var(--faint)'}">{isPaused ? 'Paused' : d.queued ? 'Queued' : eta(d.etaSec)}</span>
              <span class="dl-detail muted">{mb(d.doneBytes)} of {bytes(d.totalBytes)}</span>
              <span class="dl-pct">{pct}%</span>
              <Bar {pct} live={!isPaused && !d.queued} label={d.name} />
              <span class="dl-acts">
                <button class="btn btn-icon btn-sm btn-ghost" onclick={() => (paused[d.id] = !isPaused)} aria-label="{isPaused ? 'Resume' : 'Pause'} {d.name}"><Icon name={isPaused ? 'play' : 'pause'} size={12} /></button>
                <button class="btn btn-icon btn-sm btn-ghost" aria-label="Cancel {d.name}"><Icon name="x" size={12} sw={2} /></button>
              </span>
            </div>
            {#if open[d.id]}
              {#each d.steps as s (s.name)}
                {@const color = s.state === 'done' ? 'var(--faint)' : s.state === 'warn' ? 'var(--warn)' : 'var(--text)'}
                <div class="dl-step">
                  <span></span>
                  <span class="dl-dot" class:done={s.state === 'done'} class:warn={s.state === 'warn'} aria-hidden="true"></span>
                  <span class="trunc small" style="color: {color}">{s.name}</span>
                  <span class="dl-eta" style="color: {color}">{s.state === 'warn' ? 'Waiting' : ''}</span>
                  <span class="dl-acts">{#if s.state === 'warn'}<button class="dl-link" aria-label="Download the missing mod from CurseForge" onclick={() => session.open({ kind: 'blocked' })}>Download</button>{:else}<span class="dl-detail faint">{s.detail}</span>{/if}</span>
                  <span class="dl-pct">{s.pct}%</span>
                  <Bar pct={s.pct} color={s.state === 'warn' ? 'var(--warn)' : undefined} />
                  <span></span>
                </div>
              {/each}
            {/if}
          </div>
        {/each}
      </div>
    {:else}
      {@const label = isFinished ? 'Finished downloads' : 'Failed downloads'}
      {@const pct = percent(shownDone, shownTotal)}
      <div class="list" role="list" aria-label={label}>
        <div class="dl-row dl-sum" role="listitem" aria-label={label}>
          <span style="min-width: 0; grid-column: 1 / span 3"><span class="trunc" style="display: block; font-weight: 600">{label}</span><span class="trunc small faint tnum" style="display: block">{plural(shown.length, 'download')}</span></span>
          <span class="dl-eta" style="color: {isFinished ? 'var(--faint)' : 'var(--danger)'}">{isFinished ? 'Done' : 'Failed'}</span>
          <span class="dl-detail muted">{isFinished ? `${mb(shownDone)} MB` : `${mb(shownDone)} of ${mb(shownTotal)} MB`}</span>
          <span class="dl-pct">{pct}%</span>
          <Bar {pct} color={isFinished ? 'var(--accent)' : 'var(--danger)'} />
          <span class="dl-acts"><button class="btn btn-sm btn-fix" aria-label={isFinished ? 'Clear finished downloads' : 'Retry all failed downloads'}>{isFinished ? 'Clear' : 'Retry'}</button></span>
        </div>
        {#each shown as d (d.id)}
          <div class="dl-row" role="listitem">
            <span></span>
            <Tile art={d.art} />
            <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{d.name}</span><span class="trunc small faint" style="display: block">{d.sub}</span></span>
            <span class="dl-eta" style="color: {d.ok ? 'var(--faint)' : 'var(--danger)'}">{d.ok ? 'Done' : 'Failed'}</span>
            <span class="dl-detail muted">{d.ok ? bytes(d.totalBytes) : `${mb(d.doneBytes)} of ${bytes(d.totalBytes)}`}</span>
            <span class="dl-pct">{percent(d.doneBytes, d.totalBytes)}%</span>
            <Bar pct={percent(d.doneBytes, d.totalBytes)} color={d.ok ? 'var(--accent)' : 'var(--danger)'} />
            <span class="dl-acts">{#if !d.ok}<button class="btn btn-sm btn-fix">Retry</button>{/if}</span>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>

<style>
  /* One grid for downloads and their steps: arrow, icon, name, time left, size, percent, bar, then the row's
     buttons. Time left has its own fixed column, so a changing duration never moves the size, percent or bar. */
  .dl-row,.dl-step{display:grid;grid-template-columns:20px 32px minmax(0,1fr) 96px 140px 40px minmax(112px,21%) 60px;column-gap:14px;align-items:center;padding:0 var(--inset)}
  .dl-row{height:60px}
  .dl-sum{background:var(--raise)}
  .dl-step{height:40px;background:var(--bg);border-top:1px solid var(--line)}
  @media (max-width:1100px){.dl-row,.dl-step{grid-template-columns:20px 32px minmax(0,1fr) 80px 110px 40px minmax(84px,18%) 60px}}
  .dl-chev{width:20px;height:20px;border:0;padding:0;background:transparent;color:var(--faint);display:grid;place-items:center;border-radius:4px}
  .dl-chev:hover{color:var(--text);background:var(--raise)}
  .dl-chev :global(svg){transition:transform 160ms var(--ease)}
  .dl-chev.open :global(svg){transform:rotate(90deg)}
  .dl-detail,.dl-eta{font-size:12px;line-height:16px;text-align:right;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;min-width:0;font-variant-numeric:tabular-nums}
  .dl-acts .btn-fix{width:60px}
  /* Inline text action: underlined, same size as the step text, in the main text colour. */
  .dl-link{font-size:12px;line-height:16px;color:var(--text);text-decoration:underline;text-underline-offset:3px;text-decoration-thickness:1px;padding:4px 0;border-radius:2px;border:0;background:none}
  .dl-link:hover{text-decoration-thickness:2px}
  .dl-pct{font-family:var(--mono);font-size:12px;text-align:right;color:var(--muted);font-variant-numeric:tabular-nums}
  .dl-acts{display:flex;justify-content:flex-end;gap:4px}
  .dl-dot{width:6px;height:6px;border-radius:50%;background:var(--line-2);justify-self:center}
  .dl-dot.done{background:var(--accent)}
  .dl-dot.warn{background:var(--warn)}
</style>

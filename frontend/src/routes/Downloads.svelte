<script lang="ts">
  import type { Job, JobStep } from '$lib/api/types';
  import { bytes, count, left, percent, plural, rate, since } from '$lib/format';
  import { session } from '$lib/session.svelte';
  import { tasks } from '$lib/tasks.svelte';
  import Bar from '$lib/ui/Bar.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import PageHeader from '$lib/ui/PageHeader.svelte';
  import Tabs from '$lib/ui/Tabs.svelte';
  import Tile from '$lib/ui/Tile.svelte';
  import Empty from '$lib/ui/Empty.svelte';

  let tab = $state<0 | 1 | 2>(0);
  let open = $state<Record<string, boolean>>({});

  const list = $derived(tasks.active);
  const finished = $derived(tasks.history.filter((d) => d.state === 'done'));
  const failed = $derived(tasks.history.filter((d) => d.state === 'failed'));
  const allPaused = $derived(list.length > 0 && list.every((d) => d.state === 'paused'));
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
  // The first job starts expanded until the user toggles it.
  const isOpen = (d: Job, i: number) => open[d.id] ?? i === 0;
  const jobEta = (d: Job) => (d.state === 'paused' ? 'Paused' : d.state === 'queued' ? 'Queued' : left(d.etaSec));
  const stepPct = (s: JobStep) => percent(s.done, s.total);
  function stepDetail(s: JobStep): string {
    if (s.state === 'done') return 'Done';
    if (s.state === 'queued') return 'Queued';
    if (s.state === 'failed') return 'Failed';
    return s.unit === 'bytes' ? `${mb(s.done)} of ${bytes(s.total)}` : `${count(s.done)} of ${count(s.total)}`;
  }
  const historySub = (d: Job) => `${d.state === 'failed' ? d.error || 'Failed' : d.meta.subtitle}, ${since(d.finishedAt)}`;

  function toggleAll() {
    void tasks.all(allPaused ? 'resumeAll' : 'pauseAll');
  }
</script>

<PageHeader title="Downloads">
  {#snippet tabs()}<Tabs items={tabItems} active={tab} label="Downloads" onselect={(k) => (tab = k)} />{/snippet}
</PageHeader>
<div class="scroll">
  <div class="page" style="gap: 12px">
    {#if tab === 0 && !list.length}
      <div class="list"><Empty text="No downloads running." /></div>
    {:else if tab !== 0 && !shown.length}
      <div class="list"><Empty text={isFinished ? 'Nothing finished in the last 24 hours.' : 'No failed downloads.'} /></div>
    {:else if tab === 0}
      <div class="list" role="list" aria-label="Active downloads">
        <div class="dl-row dl-sum" role="listitem" aria-label="All downloads">
          <span style="min-width: 0; grid-column: 1 / span 3"><span class="trunc" style="display: block; font-weight: 600">All downloads</span><span class="trunc small faint tnum" style="display: block">{plural(list.length, 'download')}, {allPaused ? 'paused' : rate(tasks.speed)}</span></span>
          <span class="dl-eta" style="color: {allPaused ? 'var(--warn)' : 'var(--faint)'}">{allPaused ? 'Paused' : left(tasks.etaSec)}</span>
          <span class="dl-detail muted">{mb(sumDone)} of {mb(sumTotal)} MB</span>
          <span class="dl-pct">{sumPct}%</span>
          <Bar pct={sumPct} live={!allPaused} label="All downloads" />
          <span class="dl-acts">
            <button class="btn btn-icon btn-sm btn-ghost" onclick={toggleAll} aria-label={allPaused ? 'Resume all downloads' : 'Pause all downloads'}><Icon name={allPaused ? 'play' : 'pause'} size={12} /></button>
            <button class="btn btn-icon btn-sm btn-ghost" onclick={() => tasks.all('cancelAll')} aria-label="Cancel all downloads"><Icon name="x" size={12} sw={2} /></button>
          </span>
        </div>
        {#each list as d, i (d.id)}
          {@const isPaused = d.state === 'paused'}
          {@const queued = d.state === 'queued'}
          {@const name = d.meta.title}
          {@const pct = percent(d.doneBytes, d.totalBytes)}
          <div role="listitem">
            <div class="dl-row">
              <button class="dl-chev" class:open={isOpen(d, i)} onclick={() => (open[d.id] = !isOpen(d, i))} aria-expanded={isOpen(d, i)} aria-label="Show steps for {name}"><Icon name="chevron-right" size={12} sw={2.2} /></button>
              <Tile art={d.meta.art} icon={d.meta.art.icon} loader={d.meta.art.loader} />
              <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{name}</span><span class="trunc small faint" style="display: block">{d.meta.subtitle}</span></span>
              <span class="dl-eta" style="color: {isPaused ? 'var(--warn)' : 'var(--faint)'}">{jobEta(d)}</span>
              <span class="dl-detail muted">{mb(d.doneBytes)} of {bytes(d.totalBytes)}</span>
              <span class="dl-pct">{pct}%</span>
              <Bar {pct} live={!isPaused && !queued} label={name} />
              <span class="dl-acts">
                <button class="btn btn-icon btn-sm btn-ghost" onclick={() => tasks.run(isPaused ? 'resume' : 'pause', d.id)} aria-label="{isPaused ? 'Resume' : 'Pause'} {name}"><Icon name={isPaused ? 'play' : 'pause'} size={12} /></button>
                <button class="btn btn-icon btn-sm btn-ghost" onclick={() => tasks.run('cancel', d.id)} aria-label="Cancel {name}"><Icon name="x" size={12} sw={2} /></button>
              </span>
            </div>
            {#if isOpen(d, i)}
              {#each d.steps as s (s.name)}
                {@const color = s.state === 'done' ? 'var(--faint)' : s.state === 'warn' ? 'var(--warn)' : s.state === 'failed' ? 'var(--danger)' : 'var(--text)'}
                <div class="dl-step">
                  <span></span>
                  <span class="dl-dot" class:done={s.state === 'done'} class:warn={s.state === 'warn'} aria-hidden="true"></span>
                  <span class="trunc small" style="color: {color}">{s.name}</span>
                  <span class="dl-eta" style="color: {color}">{s.state === 'warn' ? 'Waiting' : ''}</span>
                  <span class="dl-acts">{#if s.state === 'warn'}<button class="tlink" aria-label="Download the missing mod from CurseForge" onclick={() => session.open({ kind: 'blocked', jobId: d.id })}>Download</button>{:else}<span class="dl-detail faint">{stepDetail(s)}</span>{/if}</span>
                  <span class="dl-pct">{stepPct(s)}%</span>
                  <Bar pct={stepPct(s)} color={s.state === 'warn' ? 'var(--warn)' : s.state === 'failed' ? 'var(--danger)' : undefined} />
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
          <span class="dl-acts"><button class="btn btn-sm btn-fix" onclick={() => tasks.all(isFinished ? 'clearFinished' : 'retryAllFailed')} aria-label={isFinished ? 'Clear finished downloads' : 'Retry all failed downloads'}>{isFinished ? 'Clear' : 'Retry'}</button></span>
        </div>
        {#each shown as d (d.id)}
          {@const ok = d.state === 'done'}
          <div class="dl-row" role="listitem">
            <span></span>
            <Tile art={d.meta.art} icon={d.meta.art.icon} loader={d.meta.art.loader} />
            <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{d.meta.title}</span><span class="trunc small faint" style="display: block">{historySub(d)}</span></span>
            <span class="dl-eta" style="color: {ok ? 'var(--faint)' : 'var(--danger)'}">{ok ? 'Done' : 'Failed'}</span>
            <span class="dl-detail muted">{ok ? bytes(d.totalBytes) : `${mb(d.doneBytes)} of ${bytes(d.totalBytes)}`}</span>
            <span class="dl-pct">{percent(d.doneBytes, d.totalBytes)}%</span>
            <Bar pct={percent(d.doneBytes, d.totalBytes)} color={ok ? 'var(--accent)' : 'var(--danger)'} />
            <span class="dl-acts">{#if !ok && d.retryable}<button class="btn btn-sm btn-fix" onclick={() => tasks.run('retry', d.id)} aria-label="Retry {d.meta.title}">Retry</button>{/if}</span>
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
  .dl-acts .tlink{font-size:12px;line-height:16px;padding:4px 0}
  .dl-pct{font-family:var(--mono);font-size:12px;text-align:right;color:var(--muted);font-variant-numeric:tabular-nums}
  .dl-acts{display:flex;justify-content:flex-end;gap:4px}
  .dl-dot{width:6px;height:6px;border-radius:50%;background:var(--line-2);justify-self:center}
  .dl-dot.done{background:var(--accent)}
  .dl-dot.warn{background:var(--warn)}
</style>

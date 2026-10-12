<script lang="ts">
  import { api } from '$lib/api';
  import type { RunResult } from '$lib/api/types';
  import LogsNav from '$lib/components/LogsNav.svelte';
  import { Day, clock, hm } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import { session } from '$lib/session.svelte';

  interface Props { instanceId: string }
  let { instanceId }: Props = $props();

  const runs = query(() => api.logs.runs(instanceId));
  const accounts = query(() => api.accounts.list());
  let cur = $state(0);
  const RESULT: Record<RunResult, [string, string]> = { ok: ['Closed', ''], crash: ['Crashed', 'badge-danger'], killed: ['Stopped', 'badge-warn'], failed: ['Failed to start', 'badge-danger'] };
  const run = $derived(runs.data?.[cur]);
  const when = (t: number) => `${Day(t)}, ${clock(t)}`;
  const gb = (mb: number) => Math.round((mb / 1024) * 10) / 10;
  // Runs recorded by Go carry what they used; the design's fixtures do not.
  const used = $derived(run?.java ? {
    java: run.java, memory: run.memory ? `${gb(run.memory.min)} to ${gb(run.memory.max)} GB` : '', account: run.account ?? '', sandbox: 'Off', mods: 'None',
  } : { java: '21.0.5 (Temurin)', memory: '2 to 6 GB', account: accounts.data?.find((a) => a.isDefault)?.name ?? '', sandbox: 'On', mods: '48 loaded' });
</script>

<div class="scroll">
  <div class="page" style="gap: 12px; padding-top: 14px">
    <LogsNav {instanceId} active="history" style="width: max-content" />
    <div class="lc">
      <div class="list">
        <div class="row row-head hrow"><span>Started</span><span class="opt">Length</span><span>Exit</span><span>Result</span></div>
        {#each runs.data ?? [] as r, i (r.id)}
          <button class="row hrow" class:sel={cur === i} onclick={() => (cur = i)} aria-pressed={cur === i}><span class="trunc">{when(r.startedAt)}</span><span class="mono muted opt">{hm(r.lengthMin).replace(/^(\d+) m$/, '0 h $1 m')}</span><span class="mono muted">{r.exitCode}</span><span><span class="badge {RESULT[r.result][1]}">{RESULT[r.result][0]}</span></span></button>
        {/each}
      </div>
      {#if run}
        <section class="card" aria-label="Launch details">
          <div class="card-head"><h2 class="h3">{when(run.startedAt)}</h2><span class="badge {RESULT[run.result][1]}">{RESULT[run.result][0]}</span></div>
          <div class="card-pad" style="display: flex; flex-direction: column; gap: 14px">
            <div class="kv small" style="grid-template-columns: 92px minmax(0,1fr); row-gap: 8px"><span>Java</span><span class="mono">{used.java}</span><span>Memory</span><span class="mono">{used.memory}</span><span>Account</span><span>{used.account}</span><span>Sandbox</span><span>{used.sandbox}</span><span>Mods</span><span class="tnum">{used.mods}</span></div>
            <div style="display: flex; flex-direction: column; gap: 6px">
              <h3 class="h3">Saved with this launch</h3>
              <div class="list">
                <div class="row row-sm" style="grid-template-columns: minmax(0,1fr) auto"><span class="mono trunc">latest.log</span><button class="btn btn-sm btn-ghost">Open</button></div>
                {#if run.result === 'crash'}<div class="row row-sm" style="grid-template-columns: minmax(0,1fr) auto"><span class="mono trunc" style="color: var(--danger)">crash-report.txt</span><button class="btn btn-sm btn-ghost" onclick={() => session.open({ kind: 'crash', instanceId })}>Open</button></div>{/if}
                <div class="row row-sm" style="grid-template-columns: minmax(0,1fr) auto"><span class="mono trunc">launch-command.txt</span><button class="btn btn-sm btn-ghost">Open</button></div>
              </div>
            </div>
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 8px"><button class="btn btn-sm">Copy debug info</button><button class="btn btn-sm">Upload logs</button></div>
          </div>
        </section>
      {/if}
    </div>
  </div>
</div>

<style>
  .hrow{grid-template-columns:minmax(0,1fr) 96px 96px 80px;height:44px;border:0;background:transparent;width:100%;text-align:left;color:var(--text)}
  /* Scoped .hrow outranks monolith's .row.sel, so the selected state is restated. */
  .hrow.sel{background:var(--raise)}
  @media (max-width:1100px){.hrow{grid-template-columns:minmax(0,1fr) 96px 80px}.hrow .opt{display:none}}
  .lc{display:grid;grid-template-columns:minmax(0,1fr) 340px;gap:16px;align-items:start}
  @media (max-width:1100px){.lc{grid-template-columns:minmax(0,1fr)}}
</style>

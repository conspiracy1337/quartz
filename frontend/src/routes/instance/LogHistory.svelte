<script lang="ts">
  import { api } from '$lib/api';
  import type { RunResult } from '$lib/api/types';
  import LogsFrame from '$lib/components/LogsFrame.svelte';
  import { Day, clock, hours } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import { go } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Empty from '$lib/ui/Empty.svelte';

  interface Props { instanceId: string }
  let { instanceId }: Props = $props();

  const runs = query(() => api.logs.runs(instanceId));
  // A run's details open under it; the latest crash, the run people come here to look at, starts open.
  let picked = $state<number | null | undefined>(undefined);
  const cur = $derived(picked !== undefined ? picked : (runs.data?.findIndex((r) => r.result === 'crash') ?? -1));
  const toggle = (i: number) => (picked = cur === i ? null : i);
  const RESULT: Record<RunResult, [string, string]> = { ok: ['Closed', ''], crash: ['Crashed', 'badge-danger'], killed: ['Stopped', 'badge-warn'], failed: ['Failed to start', 'badge-danger'] };
  const run = $derived(cur === null || cur < 0 ? undefined : runs.data?.[cur]);
  const when = (t: number) => `${Day(t)}, ${clock(t)}`;
  const gb = (mb: number) => Math.round((mb / 1024) * 10) / 10;
  /** Shows one of the run's saved files in the Log files viewer. */
  function open(runId: string, kind: 'log' | 'cmd', title: string) {
    session.runFile = { instanceId, runId, kind, title };
    go({ name: 'instance', id: instanceId, tab: 'logs', sub: 'files' });
  }
  const debug = async (runId: string) => session.copy(await api.logs.debugInfo(instanceId, runId), 'Debug info');
  const used = $derived({
    java: run?.java ?? '', memory: run ? `${gb(run.memory.min)} to ${gb(run.memory.max)} GB` : '', account: run?.account ?? '', sandbox: run?.sandbox ? `On (${run.sandbox})` : 'Off', mods: run?.mods ? `${run.mods} loaded` : 'None',
  });
</script>

<LogsFrame {instanceId} active="history">
  <div class="list list-fill">
    <div class="row row-head hrow"><span>Started</span><span>Exit</span><span>Result</span><span>Length</span></div>
    {#each runs.data ?? [] as r, i (r.id)}
      <button class="row hrow" class:sel={cur === i} onclick={() => toggle(i)} aria-expanded={cur === i}><span class="trunc">{when(r.startedAt)}</span><span class="mono muted">{r.exitCode}</span><span><span class="badge {RESULT[r.result][1]}">{RESULT[r.result][0]}</span></span><span class="mono muted">{hours(r.lengthMin)}</span></button>
      {#if cur === i && run}
        <section class="hd" aria-label="Launch details">
          <div class="kv small" style="grid-template-columns: 92px minmax(0,1fr); row-gap: 8px; align-content: start"><span>Java</span><span class="mono">{used.java}</span><span>Memory</span><span class="mono">{used.memory}</span><span>Account</span><span>{used.account}</span><span>Sandbox</span><span>{used.sandbox}</span><span>Mods</span><span class="tnum">{used.mods}</span></div>
          <div style="display: flex; flex-direction: column; gap: 6px">
            <h3 class="h3">Saved with this launch</h3>
            <div class="list">
              <div class="row row-sm" style="grid-template-columns: minmax(0,1fr) auto"><span class="mono trunc">latest.log</span><button class="btn btn-sm btn-ghost" onclick={() => open(run.id, 'log', 'latest.log')}>Open</button></div>
              {#if run.result === 'crash' || run.crash?.report}<div class="row row-sm" style="grid-template-columns: minmax(0,1fr) auto"><span class="mono trunc" style="color: var(--danger)">crash-report.txt</span><button class="btn btn-sm btn-ghost" onclick={() => session.open({ kind: 'crash', instanceId, runId: run.id })}>Open</button></div>{/if}
              <div class="row row-sm" style="grid-template-columns: minmax(0,1fr) auto"><span class="mono trunc">launch-command.txt</span><button class="btn btn-sm btn-ghost" onclick={() => open(run.id, 'cmd', 'launch-command.txt')}>Open</button></div>
            </div>
          </div>
          <div style="display: flex; flex-direction: column; gap: 8px; justify-content: flex-end"><button class="btn btn-sm" onclick={() => debug(run.id)}>Copy debug info</button><button class="btn btn-sm" onclick={() => session.upload(() => api.logs.uploadRun(instanceId, run.id))}>Upload logs</button></div>
        </section>
      {/if}
    {:else}
      {#if runs.data}<Empty text="No launches yet. Each launch is kept here with its log." />
      {:else}{#each { length: 6 } as _, k (k)}<div class="row hrow" aria-hidden="true"><div class="sk" style="height: 12px; width: {[60, 52, 66, 48, 58, 54][k]}%"></div><div class="sk" style="height: 10px; width: 24px"></div><div class="sk" style="height: 20px; width: 64px"></div><div class="sk" style="height: 10px; width: 56px"></div></div>{/each}{/if}
    {/each}
  </div>
</LogsFrame>

<style>
  /* Four equal columns across the full width. Buttons keep the row's height and colours. */
  .hrow{grid-template-columns:repeat(4,minmax(0,1fr))}
  button.hrow{border:0;border-top:1px solid var(--line);background:transparent;width:100%;text-align:left;color:var(--text)}
  button.hrow.sel{background:var(--raise)}
  /* A run's details, under its row. */
  .hd{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:24px;padding:16px;background:var(--bg)}
  @media (max-width:1100px){.hd{grid-template-columns:minmax(0,1fr) minmax(0,1fr)}}
</style>

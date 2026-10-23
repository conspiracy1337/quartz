<script lang="ts">
  import { api } from '../api';
  import { query } from '../query.svelte';
  import { LAUNCH_STEPS, session } from '../session.svelte';
  import { tasks } from '../tasks.svelte';
  import Bar from '../ui/Bar.svelte';
  import Icon from '../ui/Icon.svelte';
  import QueueRows from './QueueRows.svelte';

  /** The bar at the bottom of every screen while a game starts or downloads run; opens a tray with the queue.
      A starting game takes the bar's line (with Cancel); its downloads stay in the tray. */
  const t = $derived(tasks.summary);
  const launch = $derived(session.launch);
  const queue = $derived(tasks.active.length > 0);
  const instances = query(() => api.instances.list(), 'instances.list');
  const name = $derived(launch ? (instances.data?.find((i) => i.id === launch.instanceId)?.name ?? '') : '');
  const label = $derived(launch ? `Starting ${name} · ${LAUNCH_STEPS[launch.stage] ?? ''}` : t.label);
  const pct = $derived(launch ? (launch.pct ?? Math.round((launch.stage / LAUNCH_STEPS.length) * 100)) : t.pct);
  const meta = $derived(launch ? (launch.details?.[launch.stage] ?? '') : t.speed);
</script>

{#if launch || queue}
  {#if queue && session.downloadTray}
    <section class="tray" aria-label="Download queue"><QueueRows /></section>
  {/if}
  <div class="sbar" role={launch ? 'status' : undefined}>
    <button class="sbar-main" disabled={!queue} onclick={() => (session.downloadTray = !session.downloadTray)} aria-expanded={queue ? session.downloadTray : undefined}>
      {#if launch}<Icon name="spinner" size={14} sw={2.4} class="spin" color="var(--accent)" />{:else}<Icon name="download" size={14} sw={2} color="var(--accent)" />{/if}
      <span class="small trunc">{label}</span>
      <Bar {pct} style="width: 160px; flex: 0 0 auto" />
      <span class="mono small faint tnum">{pct}%</span>
      <span class="mono small faint" style="margin-left: auto">{meta}</span>
      {#if queue}<Icon name="chevron-up" size={12} sw={2} style="transform: rotate({session.downloadTray ? 180 : 0}deg); transition: transform 160ms cubic-bezier(.23,1,.32,1)" />{/if}
    </button>
    {#if launch}<button class="btn btn-sm btn-ghost" onclick={() => session.cancelLaunch()}>Cancel</button>{/if}
  </div>
{/if}

<style>
  .sbar{flex:0 0 36px;height:36px;display:flex;align-items:center;gap:8px;padding-right:10px;border-top:1px solid var(--line);background:var(--side)}
  .sbar-main{flex:1 1 auto;min-width:0;align-self:stretch;display:flex;align-items:center;gap:12px;padding:0 6px 0 28px;border:0;background:none;text-align:left;color:var(--text)}
  .sbar:has(.sbar-main:enabled:hover){background:var(--raise)}
  .sbar-main:disabled{cursor:default}
  .tray{flex:0 0 auto;border-top:1px solid var(--line);background:var(--panel);overflow:hidden}
  @media (prefers-reduced-motion:no-preference){.tray{animation:up 200ms cubic-bezier(.23,1,.32,1)}}
  @keyframes up{from{transform:translateY(12px);opacity:0}to{transform:none;opacity:1}}
</style>

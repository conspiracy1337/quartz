<script lang="ts">
  import { api } from '../api';
  import { hasBridge } from '../bridge';
  import { revs } from '../revs.svelte';
  import { query } from '../query.svelte';
  import { LAUNCH_STEPS, session } from '../session.svelte';
  import Bar from '../ui/Bar.svelte';
  import Icon from '../ui/Icon.svelte';
  import Tile from '../ui/Tile.svelte';

  /** Bottom-right launch progress with each step. Inside the app Go sends each step's
      detail and the overall percent; the preview shows the design's sample values. */
  interface Props { instanceId: string; accountId?: string; stage: number; details?: string[] | undefined; pct?: number | undefined }
  let { instanceId, accountId = '', stage, details, pct: realPct }: Props = $props();
  const inst = query(() => api.instances.get(instanceId));
  const accounts = query(() => api.accounts.list());
  const me = $derived(accounts.data?.find((a) => (accountId ? a.id === accountId : a.isDefault))?.name ?? '');
  // Before Go's first progress event the app shows bare step names; only the preview fills in samples.
  const sample = $derived(!details && !hasBridge());
  const META = $derived(details ?? (sample ? [`0.3 s`, '0.4 s', 'ready', '2,318 / 3,604', 'bubblewrap', ''] : []));
  const names = $derived(LAUNCH_STEPS.map((s, i) => (i === 0 ? `Signing in as ${me}` : i === 1 && sample ? `Checking ${inst.data?.counts.mods ?? ''} mods` : i === 2 && sample ? 'Java 21.0.5' : s)));
  const pct = $derived(realPct ?? Math.round((stage / LAUNCH_STEPS.length) * 100));
</script>

<section class="lp" role="status" aria-label="Launching {inst.data?.name ?? ''}">
  <div style="display: grid; grid-template-columns: 32px minmax(0,1fr) auto; gap: 10px; align-items: center; padding: 14px 14px 10px">
    {#if inst.data}<Tile art={inst.data.art} icon={inst.data.icon} />{:else}<span></span>{/if}
    <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">Starting {inst.data?.name ?? ''}</span><span class="small faint tnum">{details ? `${pct}% ready` : sample ? `About ${Math.max(1, LAUNCH_STEPS.length - stage)} seconds` : 'Starting…'}</span></span>
    <button class="btn btn-sm btn-ghost" onclick={() => session.cancelLaunch()}>Cancel</button>
  </div>
  <div style="margin: 0 14px"><Bar {pct} label="Launch progress" /></div>
  <div style="padding: 10px 14px 14px">
    {#each names as name, i (i)}
      <div class="step">
        {#if i < stage}<Icon name="check" size={14} sw={2.6} class="ok" />{:else if i === stage}<Icon name="spinner" size={14} sw={2.4} class="spin" color="var(--text)" />{:else}<span class="wait" style="width: 6px; height: 6px; border-radius: 50%; background: currentColor; justify-self: center" aria-hidden="true"></span>{/if}
        <span class="trunc" style="color: {i > stage ? 'var(--faint)' : 'var(--text)'}">{name}</span>
        <span class="mono small faint">{i <= stage ? (META[i] ?? '') : ''}</span>
      </div>
    {/each}
  </div>
</section>

<style>
  .lp{position:absolute;right:20px;bottom:20px;width:340px;border:1px solid var(--line-2);border-radius:12px;background:var(--raise);box-shadow:0 24px 64px -16px rgba(0,0,0,.8);z-index:10}
  @media (prefers-reduced-motion:no-preference){.lp{animation:in 220ms cubic-bezier(.23,1,.32,1)}}
  @keyframes in{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:none}}
  .step{display:grid;grid-template-columns:18px minmax(0,1fr) auto;gap:10px;align-items:center;height:30px}
  .step :global(.ok){color:var(--accent)} .wait{color:var(--faint)}
</style>

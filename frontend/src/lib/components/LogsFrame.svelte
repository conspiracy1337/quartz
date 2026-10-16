<script lang="ts">
  import type { Snippet } from 'svelte';
  import { href } from '../router.svelte';

  /**
   * The Logs tab's one frame (LogsD.dc.html): a bar with the Live / Files / History switch
   * over the rail column, so it sits in the same place on every view, and the view's tools
   * over the main column. Live and Files pass a rail (sources, files); History has none and
   * takes the full width.
   */
  interface Props { instanceId: string; active: 'live' | 'files' | 'history'; tools?: Snippet; rail?: Snippet; children: Snippet }
  let { instanceId, active, tools, rail, children }: Props = $props();
  const VIEWS = [{ key: 'live', label: 'Live' }, { key: 'files', label: 'Files' }, { key: 'history', label: 'History' }] as const;
  const to = (k: string) => href({ name: 'instance', id: instanceId, tab: 'logs', ...(k === 'live' ? {} : { sub: k }) });
</script>

<div class="lf">
  <div class="lf-grid lf-bar">
    <nav class="seg lf-nav" aria-label="Logs">
      {#each VIEWS as v (v.key)}<a href={to(v.key)} class:on={v.key === active} aria-current={v.key === active ? 'page' : undefined}>{v.label}</a>{/each}
    </nav>
    <div class="lf-tools">{@render tools?.()}</div>
  </div>
  <div class="lf-body" class:lf-grid={!!rail}>
    {#if rail}<div class="lf-rail scroll">{@render rail()}</div>{/if}
    <div class="lf-main">{@render children()}</div>
  </div>
</div>

<style>
  .lf{flex:1 1 auto;min-height:0;display:flex;flex-direction:column}
  /* The bar and the body share one grid, so the tools start where the console or file starts. */
  .lf-grid{display:grid;grid-template-columns:240px minmax(0,1fr);column-gap:var(--gap)}
  .lf-bar{align-items:center;min-height:58px;padding:14px 28px 12px}
  .lf-nav{display:flex}
  .lf-nav a{flex:1 1 0;justify-content:center;text-decoration:none}
  .lf-tools{display:flex;align-items:center;gap:8px;min-width:0}
  .lf-body{flex:1 1 auto;min-height:0;padding:0 28px 20px;display:flex;flex-direction:column}
  .lf-body.lf-grid{display:grid}
  .lf-rail{display:flex;flex-direction:column;gap:2px;min-height:0}
  .lf-main{display:flex;flex-direction:column;min-width:0;min-height:0;flex:1 1 auto}
  @media (max-width:1100px){.lf-grid{grid-template-columns:200px minmax(0,1fr)}}
</style>

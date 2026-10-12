<script lang="ts">
  import { api } from '../api';
  import { query } from '../query.svelte';
  import Bar from '../ui/Bar.svelte';
  import Icon from '../ui/Icon.svelte';
  import Tile from '../ui/Tile.svelte';

  /** The bar at the bottom of every screen while downloads run; opens a tray with the queue. */
  const tray = query(() => api.downloads.tray());
  let open = $state(true);
</script>

{#if tray.data}
  {@const t = tray.data}
  {#if open}
    <section class="tray" aria-label="Download queue">
      {#each t.items as it (it.name)}
        <div class="tr"><Tile art={it.art} size="sm" style="width: 24px; height: 24px" /><span class="trunc">{it.name}</span><Bar pct={it.pct ?? 0} /><span class="small faint" class:mono={it.pct !== null} style="text-align: right">{it.pct === null ? 'Queued' : `${it.pct}%`}</span></div>
      {/each}
    </section>
  {/if}
  <button class="sbar" onclick={() => (open = !open)} aria-expanded={open}>
    <Icon name="download" size={14} sw={2} color="var(--accent)" />
    <span class="small">{t.label}</span>
    <Bar pct={t.pct} style="width: 160px" />
    <span class="mono small faint">{t.pct}%</span>
    <span class="mono small faint" style="margin-left: auto">{t.speed}</span>
    <Icon name="chevron-up" size={12} sw={2} style="transform: rotate({open ? 180 : 0}deg); transition: transform 160ms cubic-bezier(.23,1,.32,1)" />
  </button>
{/if}

<style>
  .sbar{flex:0 0 36px;height:36px;display:flex;align-items:center;gap:12px;padding:0 16px 0 28px;border-top:1px solid var(--line);background:var(--side);border-left:0;border-right:0;border-bottom:0;width:100%;text-align:left;color:var(--text)}
  .sbar:hover{background:var(--raise)}
  .tray{flex:0 0 auto;border-top:1px solid var(--line);background:var(--panel);overflow:hidden}
  @media (prefers-reduced-motion:no-preference){.tray{animation:up 200ms cubic-bezier(.23,1,.32,1)}}
  @keyframes up{from{transform:translateY(12px);opacity:0}to{transform:none;opacity:1}}
  .tr{display:grid;grid-template-columns:24px minmax(0,1fr) 200px 56px;gap:12px;align-items:center;height:44px;padding:0 16px 0 28px}
  @media (max-width:1100px){.tr{grid-template-columns:24px minmax(0,1fr) 120px 48px}}
</style>

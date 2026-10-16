<script lang="ts">
  import { tasks } from '../tasks.svelte';
  import Bar from '../ui/Bar.svelte';
  import Tile from '../ui/Tile.svelte';

  /** One row per running download (the status bar's tray and the download panel). `compact` fits the panel's width. */
  interface Props { compact?: boolean }
  let { compact = false }: Props = $props();
</script>

{#each tasks.summary.items as it (it.id)}
  <div class="tr" class:compact><Tile art={it.art} size="sm" style="width: 24px; height: 24px" /><span class="trunc">{it.name}</span><Bar pct={it.pct ?? 0} /><span class="small faint" class:mono={it.pct !== null} style="text-align: right">{it.pct === null ? 'Queued' : `${it.pct}%`}</span></div>
{/each}

<style>
  .tr{display:grid;grid-template-columns:24px minmax(0,1fr) 200px 56px;gap:12px;align-items:center;height:44px;padding:0 16px 0 28px}
  @media (max-width:1100px){.tr{grid-template-columns:24px minmax(0,1fr) 120px 48px}}
  .tr.compact{grid-template-columns:24px minmax(0,1fr) 72px 44px;gap:10px;height:40px;padding:0 14px}
</style>

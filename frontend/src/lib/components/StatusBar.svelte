<script lang="ts">
  import { session } from '../session.svelte';
  import { tasks } from '../tasks.svelte';
  import Bar from '../ui/Bar.svelte';
  import Icon from '../ui/Icon.svelte';
  import QueueRows from './QueueRows.svelte';

  /** The bar at the bottom of every screen while downloads run; opens a tray with the queue. */
  const t = $derived(tasks.summary);
</script>

{#if tasks.active.length}
  {#if session.downloadTray}
    <section class="tray" aria-label="Download queue"><QueueRows /></section>
  {/if}
  <button class="sbar" onclick={() => (session.downloadTray = !session.downloadTray)} aria-expanded={session.downloadTray}>
    <Icon name="download" size={14} sw={2} color="var(--accent)" />
    <span class="small">{t.label}</span>
    <Bar pct={t.pct} style="width: 160px" />
    <span class="mono small faint">{t.pct}%</span>
    <span class="mono small faint" style="margin-left: auto">{t.speed}</span>
    <Icon name="chevron-up" size={12} sw={2} style="transform: rotate({session.downloadTray ? 180 : 0}deg); transition: transform 160ms cubic-bezier(.23,1,.32,1)" />
  </button>
{/if}

<style>
  .sbar{flex:0 0 36px;height:36px;display:flex;align-items:center;gap:12px;padding:0 16px 0 28px;border-top:1px solid var(--line);background:var(--side);border-left:0;border-right:0;border-bottom:0;width:100%;text-align:left;color:var(--text)}
  .sbar:hover{background:var(--raise)}
  .tray{flex:0 0 auto;border-top:1px solid var(--line);background:var(--panel);overflow:hidden}
  @media (prefers-reduced-motion:no-preference){.tray{animation:up 200ms cubic-bezier(.23,1,.32,1)}}
  @keyframes up{from{transform:translateY(12px);opacity:0}to{transform:none;opacity:1}}
</style>

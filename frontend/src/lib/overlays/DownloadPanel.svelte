<script lang="ts">
  import { session } from '../session.svelte';
  import { tasks } from '../tasks.svelte';
  import Bar from '../ui/Bar.svelte';
  import Icon from '../ui/Icon.svelte';
  import QueueRows from '../components/QueueRows.svelte';

  /** Bottom-right queue after an install from Browse or a project page, composed from the
      server panel's header and the status bar's tray: the page stays where it is. */
  const t = $derived(tasks.summary);
</script>

<section class="float dp" aria-label="Downloads">
  <div style="display: flex; align-items: center; gap: 10px; padding: 12px 10px 10px 14px">
    <Icon name="download" size={14} sw={2} color="var(--accent)" />
    <span class="trunc" style="font-weight: 500; margin-right: auto">{t.label}</span>
    <span class="mono small faint">{t.speed}</span>
    <button class="btn-bare" aria-label="Close panel" title="Close" onclick={() => (session.downloadPanel = false)}><Icon name="x" size={12} sw={2} /></button>
  </div>
  <div style="margin: 0 14px"><Bar pct={t.pct} label="Download progress" /></div>
  <div class="rows"><QueueRows compact /></div>
</section>

<style>
  .dp{width:340px}
  .rows{padding:6px 0 8px;max-height:220px;overflow-y:auto;overscroll-behavior:contain}
</style>

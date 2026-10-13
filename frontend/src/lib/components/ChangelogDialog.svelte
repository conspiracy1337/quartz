<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ChangelogVersion } from '../api/types';
  import { shortDate } from '../format';
  import Dialog from '../ui/Dialog.svelte';

  /** A changelog: each version's id, date and lines (mod updates; the launcher's own update). */
  interface Props {
    title: string;
    /** The mono line under the title, such as "1.8.1 to 1.8.3". */
    meta: string;
    versions: ChangelogVersion[];
    onclose: () => void;
    foot: Snippet;
  }
  let { title, meta, versions, onclose, foot }: Props = $props();
</script>

<Dialog {title} {onclose} {foot}>
  {#snippet head()}<span><span class="h2" style="display: block">{title}</span><span class="mono small faint">{meta}</span></span>{/snippet}
  {#each versions as v (v.id)}
    <section aria-label={v.id} style="display: flex; flex-direction: column; gap: 6px">
      <div style="display: flex; justify-content: space-between; align-items: baseline"><span class="mono" style="font-weight: 500">{v.id}</span><span class="small faint">{shortDate(v.date)}</span></div>
      <ul class="cl">{#each v.items as t (t)}<li>{t}</li>{/each}</ul>
    </section>
  {/each}
</Dialog>

<style>
  .cl{margin:0;padding-left:18px;color:var(--muted);line-height:22px}
</style>

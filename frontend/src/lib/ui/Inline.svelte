<script lang="ts">
  import { api } from '../api';
  import type { Span } from '../api/types';

  /** Description text: plain runs, <code>, and links that open the browser. Renders text nodes only, never HTML. */
  interface Props { spans: Span[] }
  let { spans }: Props = $props();
</script>

{#each spans as s, i (i)}{#if s.code}<code>{s.text}</code>{:else if s.href}{@const url = s.href}<button class="tlink" onclick={() => void api.app.openUrl(url)}>{s.text}</button>{:else}{s.text}{/if}{/each}

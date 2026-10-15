<script lang="ts">
  import type { Art } from '../api/types';

  /** What sits inside an instance tile: the chosen picture, else the letter.
      The tile itself (.ico) is the caller's markup. */
  interface Props { art: Art; icon?: string | undefined }
  let { art, icon }: Props = $props();
  const picture = $derived(!!icon && /^[0-9a-f]{64}$/.test(icon));
</script>

{#if picture}<img class="ico-pic" src="/tex/icon/{icon}.png" alt="" draggable="false" />{:else}{art.initial ?? ''}{/if}

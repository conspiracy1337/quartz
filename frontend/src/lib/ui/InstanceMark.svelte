<script lang="ts">
  import type { Art } from '../api/types';
  import { isPicture, isServed } from '../look';

  /** What sits inside an instance tile: the chosen picture (a stored one, or one Go serves
      such as a modpack's icon before install), else the letter. The tile itself (.ico) is
      the caller's markup. */
  interface Props { art: Art; icon?: string | undefined }
  let { art, icon }: Props = $props();
  const src = $derived(isPicture(icon) ? `/tex/icon/${icon}.png` : isServed(icon) ? icon : '');
</script>

{#if src}<img class="ico-pic" {src} alt="" draggable="false" />{:else}{art.initial ?? ''}{/if}

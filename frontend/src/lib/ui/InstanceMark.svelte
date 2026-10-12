<script lang="ts">
  import type { Art } from '../api/types';
  import Icon from './Icon.svelte';
  import { icons, type IconName } from './icons';

  /** What sits inside an instance tile: the chosen picture, else the letter. The
      instance header alone draws the design's glyph icons (`glyphs`). The tile
      itself (.ico) is the caller's markup. */
  interface Props { art: Art; icon?: string | undefined; glyphs?: boolean; glyph?: number }
  let { art, icon, glyphs = false, glyph = 24 }: Props = $props();
  const picture = $derived(!!icon && /^[0-9a-f]{64}$/.test(icon));
</script>

{#if picture}<img class="ico-pic" src="/tex/icon/{icon}.png" alt="" draggable="false" />{:else if glyphs && icon && icon in icons}<Icon name={icon as IconName} size={glyph} sw={1.8} />{:else}{art.initial ?? ''}{/if}

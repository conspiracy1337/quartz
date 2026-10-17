<script lang="ts">
  import type { Art } from '../api/types';
  import { isModLoader } from '../labels';
  import { isPicture, isServed } from '../look';
  import LoaderIcon from './LoaderIcon.svelte';

  /** The tile of an instance or project. A letter fills it with the tile colour. A picture
      (a stored one, or one Go serves such as a modpack's icon before install) shows as it
      is, framed by that colour; so does the loader's mark, which an instance (one passed
      `loader`, "none" or "" for vanilla) shows when it has neither. */
  interface Props { art: Art; icon?: string | undefined; loader?: string | undefined; size?: 'sm' | 'md' | 'lg' | 'xl'; style?: string }
  let { art, icon, loader, size = 'md', style = '' }: Props = $props();
  const sizeClass = { sm: 'ico-sm', md: '', lg: 'ico-lg', xl: 'ico-xl' } as const;
  const src = $derived(isPicture(icon) ? `/tex/icon/${icon}.png` : isServed(icon) ? icon : '');
  const mark = $derived(!src && !art.initial && loader !== undefined);
  const framed = $derived(!!src || mark);
</script>

<span class="ico {sizeClass[size]}" class:ico-framed={framed} style="{framed ? `--tint: ${art.background}` : `background: ${art.background}`}; {style}" aria-hidden="true">
  {#if src}<img class="ico-pic" {src} alt="" draggable="false" />
  {:else if mark}<span class="ico-mark"><LoaderIcon kind={loader && isModLoader(loader) ? loader : ''} /></span>
  {:else}{art.initial ?? ''}{/if}
</span>

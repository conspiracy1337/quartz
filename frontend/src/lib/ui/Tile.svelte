<script lang="ts">
  import type { Art } from '../api/types';

  /** A coloured .ico tile with an optional letter. */
  interface Props { art: Art; size?: 'sm' | 'md' | 'lg' | 'xl'; class?: string; style?: string }
  let { art, size = 'md', class: cls = '', style = '' }: Props = $props();
  const sizeClass = { sm: 'ico-sm', md: '', lg: 'ico-lg', xl: 'ico-xl' } as const;
</script>

<span class="ico {sizeClass[size]} {cls}" style="background: {art.background}; {style}" aria-hidden="true">{art.initial ?? ''}</span>

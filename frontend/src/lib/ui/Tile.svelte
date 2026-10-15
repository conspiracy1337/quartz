<script lang="ts">
  import type { Art } from '../api/types';
  import InstanceMark from './InstanceMark.svelte';

  /** A coloured .ico tile with an optional letter, or an instance's picture. */
  interface Props { art: Art; icon?: string | undefined; size?: 'sm' | 'md' | 'lg' | 'xl'; style?: string }
  let { art, icon, size = 'md', style = '' }: Props = $props();
  const sizeClass = { sm: 'ico-sm', md: '', lg: 'ico-lg', xl: 'ico-xl' } as const;
</script>

<span class="ico {sizeClass[size]}" style="background: {art.background}; {style}" aria-hidden="true"><InstanceMark {art} {icon} /></span>

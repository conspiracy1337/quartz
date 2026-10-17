<script lang="ts">
  import grassBlock from '$assets/grass-block.png';
  import type { Loader } from '../api/types';
  import Icon from './Icon.svelte';

  /** A mod loader's mark in its colour; no loader (vanilla) is Minecraft's own grass block. */
  interface Props { kind: Loader | 'none' | ''; size?: number; class?: string }
  let { kind, size = 16, class: cls = '' }: Props = $props();
</script>

{#if kind && kind !== 'none'}
  <Icon name={kind} {size} sw={2} color="var(--{kind})" class={cls} />
{:else}
  <img src={grassBlock} width={size} height={size} class="px {cls}" alt="" aria-hidden="true" draggable="false" />
{/if}

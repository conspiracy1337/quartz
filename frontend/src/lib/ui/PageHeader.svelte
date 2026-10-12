<script lang="ts">
  import type { Snippet } from 'svelte';

  /** A page title row over an optional tab strip (.phead), built like the instance header. */
  interface Props { title: string; style?: string; actions?: Snippet; tabs?: Snippet }
  let { title, style, actions, tabs }: Props = $props();
</script>

<header class="phead" {style}>
  <div class="phead-row"><h1 class="h1">{title}</h1>{@render actions?.()}</div>
  {@render tabs?.()}
</header>

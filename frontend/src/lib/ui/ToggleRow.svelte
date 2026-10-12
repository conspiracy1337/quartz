<script lang="ts">
  import type { Snippet } from 'svelte';
  import Toggle from './Toggle.svelte';

  /** One .tgl row in a .tgl-grid: label with an optional mono hint, optional count, then a toggle. */
  interface Props {
    name: string;
    hint?: string;
    count?: string;
    on: boolean;
    disabled?: boolean;
    title?: string;
    /** Shows the hint alone as the label (custom paths). */
    pathOnly?: boolean;
    onchange?: (on: boolean) => void;
    after?: Snippet;
  }
  let { name, hint, count, on = $bindable(), disabled = false, title, pathOnly = false, onchange, after }: Props = $props();
</script>

<div class="tgl" class:off={disabled} {title}>
  {#if pathOnly}
    <span class="trunc mono" style="margin: 0; color: var(--text)">{hint}</span>
  {:else}
    <span class="trunc">{name}{#if hint}<span class="mono">{hint}</span>{/if}</span>
  {/if}
  {#if count !== undefined}<span class="count">{count}</span>{/if}
  <Toggle bind:on label="Sync {name || hint}" {disabled} {onchange} />
  {@render after?.()}
</div>

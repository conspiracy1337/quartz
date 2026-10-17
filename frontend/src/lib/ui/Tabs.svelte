<script lang="ts" generics="K extends string | number">
  import type { Snippet } from 'svelte';

  /** The one tab strip (.tabs). Items with an href navigate; the others call onselect; end follows the last tab. */
  interface Item { key: K; label: string; count?: number | undefined; href?: string }
  interface Props { items: readonly Item[]; active: K | undefined; label: string; style?: string; onselect?: (k: K) => void; end?: Snippet }
  let { items, active, label, style, onselect, end }: Props = $props();
</script>

<nav class="tabs" aria-label={label} {style} role={onselect ? 'tablist' : undefined}>
  {#each items as t (t.key)}
    {#if t.href}
      <a href={t.href} class="tab" class:on={t.key === active} aria-current={t.key === active ? 'page' : undefined}>{t.label}{#if t.count !== undefined}{' '}<span class="count">{t.count}</span>{/if}</a>
    {:else}
      <button class="tab" class:on={t.key === active} role="tab" aria-selected={t.key === active} onclick={() => onselect?.(t.key)}>{t.label}{#if t.count !== undefined}{' '}<span class="count">{t.count}</span>{/if}</button>
    {/if}
  {/each}
  {#if end}<span class="tabs-end">{@render end()}</span>{/if}
</nav>

<style>
  .tabs-end{display:flex;align-items:center;flex:none}
</style>

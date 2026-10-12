<script lang="ts" generics="T extends string">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  /** A .select button that opens the shared .menu. */
  interface Props {
    value: T;
    options: readonly { value: T; label: string }[];
    label: string;
    style?: string;
    /** Optional leading content inside the button, e.g. an instance tile. */
    lead?: Snippet;
    onchange?: (v: T) => void;
  }
  let { value = $bindable(), options, label, style = '', lead, onchange }: Props = $props();
  let open = $state(false);
  let root: HTMLElement;
  const current = $derived(options.find((o) => o.value === value)?.label ?? value);

  function pick(v: T) {
    open = false;
    value = v;
    onchange?.(v);
  }
  function outside(e: PointerEvent) {
    if (open && !root.contains(e.target as Node)) open = false;
  }
  function keys(e: KeyboardEvent) {
    if (e.key === 'Escape' && open) {
      e.stopPropagation();
      open = false;
    }
  }
</script>

<svelte:window onpointerdown={outside} />

<span class="sel" bind:this={root} onkeydown={keys} role="presentation" {style}>
  <button class="select" style="width: 100%; height: inherit" aria-haspopup="listbox" aria-expanded={open} aria-label={label} onclick={() => (open = !open)}>
    {#if lead}{@render lead()}{/if}
    <span class="trunc">{current}</span>
    <Icon name="chevron-down" size={12} sw={2} />
  </button>
  {#if open}
    <div class="menu sel-menu" role="listbox" aria-label={label}>
      {#each options as o (o.value)}
        <button class="menu-item" role="option" aria-selected={o.value === value} onclick={() => pick(o.value)}>
          <span>{#if o.value === value}<Icon name="check" size={14} sw={2.4} />{/if}</span>
          <span class="trunc">{o.label}</span>
          <span></span>
        </button>
      {/each}
    </div>
  {/if}
</span>

<style>
  .sel{position:relative;display:inline-flex;min-width:0;height:32px}
  .sel-menu{position:absolute;top:calc(100% + 4px);left:0;right:0;z-index:30;max-height:280px;overflow:auto}
</style>

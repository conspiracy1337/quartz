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
    /** Draws the chosen option inside the button (default: its label as text). */
    current?: Snippet<[T]>;
    /** Places the menu over the page, for a select inside a clipping list. */
    fixed?: boolean;
    onchange?: (v: T) => void;
  }
  let { value = $bindable(), options, label, style = '', lead, current: shown, fixed = false, onchange }: Props = $props();
  let open = $state(false);
  let root: HTMLElement;
  let at = $state('');

  function toggle() {
    if (!open && fixed) {
      const r = root.getBoundingClientRect();
      // Anchored to the button's right edge, so a menu wider than the
      // button grows inwards, never past the window.
      at = `position: fixed; top: ${r.bottom + 4}px; right: ${window.innerWidth - r.right}px; left: auto; min-width: ${r.width}px`;
    }
    open = !open;
  }
  // A fixed menu would drift from its button, so scrolling or resizing closes it.
  $effect(() => {
    if (!open || !fixed) return;
    const close = (e: Event) => {
      if (!(e.target instanceof Node) || !root.contains(e.target)) open = false;
    };
    window.addEventListener('scroll', close, true);
    window.addEventListener('resize', close);
    return () => {
      window.removeEventListener('scroll', close, true);
      window.removeEventListener('resize', close);
    };
  });
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
  <button class="select" style="width: 100%; height: inherit" aria-haspopup="listbox" aria-expanded={open} aria-label={label} onclick={toggle}>
    {#if lead}{@render lead()}{/if}
    {#if shown}{@render shown(value)}{:else}<span class="trunc">{current}</span>{/if}
    <Icon name="chevron-down" size={12} sw={2} />
  </button>
  {#if open}
    <div class="menu sel-menu" role="listbox" aria-label={label} style={fixed ? at : undefined}>
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

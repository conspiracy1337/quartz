<script lang="ts">
  import type { Snippet } from 'svelte';

  /**
   * A popup menu at a point (a right-click, or under a button), kept inside the window.
   * Escape, a click elsewhere, scrolling or resizing closes it. Rows are `.mi` buttons
   * with role="menuitem"; the first one gets focus.
   */
  interface Props { x: number; y: number; label: string; title?: string | undefined; onclose: () => void; children: Snippet }
  let { x, y, label, title, onclose, children }: Props = $props();
  let el = $state<HTMLElement>();
  let size = $state({ w: 0, h: 0 });

  const left = $derived(Math.max(8, Math.min(x, window.innerWidth - size.w - 8)));
  const top = $derived(Math.max(8, Math.min(y, window.innerHeight - size.h - 8)));

  $effect(() => {
    if (!el) return;
    size = { w: el.offsetWidth, h: el.offsetHeight };
    el.querySelector<HTMLElement>('[role="menuitem"]')?.focus();
  });

  // Inner panes scroll without bubbling, so listen in the capture phase.
  $effect(() => {
    const close = () => onclose();
    window.addEventListener('scroll', close, true);
    return () => window.removeEventListener('scroll', close, true);
  });

  function keys(e: KeyboardEvent) {
    if (e.key === 'Escape') return onclose();
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
    e.preventDefault();
    const items = [...(el?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])];
    const i = items.indexOf(document.activeElement as HTMLElement);
    items[(i + (e.key === 'ArrowDown' ? 1 : items.length - 1)) % items.length]?.focus();
  }
  const outside = (e: PointerEvent) => {
    if (!el?.contains(e.target as Node)) onclose();
  };
</script>

<svelte:window onkeydown={keys} onpointerdown={outside} onresize={onclose} onblur={onclose} />
<div class="menu um" role="menu" aria-label={label} bind:this={el} style="left: {left}px; top: {top}px">
  {#if title}<div class="small faint menu-title trunc">{title}</div>{/if}
  {@render children()}
</div>

<style>
  .um{position:fixed;z-index:50;min-width:240px;max-width:320px}
</style>

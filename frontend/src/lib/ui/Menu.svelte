<script lang="ts" module>
  // The button the next menu opens under (see below), while that menu is about to mount.
  let opener: { el: HTMLElement; at: number } | null = null;
  const OPENER_MS = 2000;

  /** A menu's place under the button that opens it, right-aligned to it. Pressing that
      button again while the menu shows closes it. */
  export function below(e: MouseEvent) {
    const el = e.currentTarget as HTMLElement;
    opener = { el, at: performance.now() };
    const r = el.getBoundingClientRect();
    return { x: r.right - 240, y: r.bottom + 4 };
  }

  /** The button a menu mounting now was opened from; a stale one (a below() whose menu never came) is dropped. */
  function takeOpener(): HTMLElement | null {
    const o = opener;
    opener = null;
    return o && o.el.isConnected && performance.now() - o.at < OPENER_MS ? o.el : null;
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';

  /**
   * A popup menu at a point (a right-click, or under a button), kept inside the window.
   * Escape, a click elsewhere, scrolling or resizing closes it. Rows are `.mi` buttons
   * with role="menuitem"; the first one gets focus.
   */
  /** scroll caps a long menu at the height the Select menu uses and scrolls inside it. */
  interface Props { x: number; y: number; label: string; title?: string | undefined; scroll?: boolean; onclose: () => void; children: Snippet }
  let { x, y, label, title, scroll = false, onclose, children }: Props = $props();
  let el = $state<HTMLElement>();
  // Bound sizes follow the menu as rows arrive (InstanceMenu's accounts, Syncing's
  // key list), so the clamp always uses the real size, not the first empty one.
  let w = $state(0);
  let h = $state(0);

  const left = $derived(Math.max(8, Math.min(x, window.innerWidth - w - 8)));
  const top = $derived(Math.max(8, Math.min(y, window.innerHeight - h - 8)));

  // The first item takes focus, again when rows arrive above it (the size changes),
  // unless the player has already moved to another item.
  let focused: HTMLElement | null = null;
  $effect(() => {
    void h;
    const first = el?.querySelector<HTMLElement>('[role="menuitem"]');
    if (!el || !first || first === focused) return;
    if (focused && document.activeElement !== focused && el.contains(document.activeElement)) return;
    first.focus();
    focused = first;
  });

  // Inner panes scroll without bubbling, so listen in the capture phase (but not to
  // the menu's own scrolling).
  $effect(() => {
    const close = (e: Event) => {
      if (!el?.contains(e.target as Node)) onclose();
    };
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
  // Its opener shows the menu open while it is.
  const anchor = takeOpener();
  $effect(() => {
    anchor?.setAttribute('aria-expanded', 'true');
    return () => anchor?.setAttribute('aria-expanded', 'false');
  });

  /** Eats the click that follows a press on the opener, so it doesn't open the menu again. */
  function swallowClick(on: HTMLElement) {
    const eat = (e: MouseEvent) => {
      if (on.contains(e.target as Node)) {
        e.stopPropagation();
        e.preventDefault();
      }
    };
    window.addEventListener('click', eat, true);
    // The click comes right after the release; a press dragged away never clicks.
    window.addEventListener('pointerup', () => setTimeout(() => window.removeEventListener('click', eat, true)), { capture: true, once: true });
  }

  const outside = (e: PointerEvent) => {
    if (el?.contains(e.target as Node)) return;
    if (anchor?.contains(e.target as Node)) swallowClick(anchor);
    onclose();
  };
</script>

<svelte:window onkeydown={keys} onpointerdown={outside} onresize={onclose} onblur={onclose} />
<div class="menu um" class:um-scroll={scroll} role="menu" aria-label={label} bind:this={el} bind:offsetWidth={w} bind:offsetHeight={h} style="left: {left}px; top: {top}px">
  {#if title}<div class="small faint menu-title trunc">{title}</div>{/if}
  {@render children()}
</div>

<style>
  .um{position:fixed;z-index:50;min-width:240px;max-width:320px;max-height:calc(100vh - 16px);overflow:auto}
  .um-scroll{max-height:280px;overflow:auto}
</style>

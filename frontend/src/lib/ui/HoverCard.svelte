<script lang="ts">
  import type { Snippet } from 'svelte';

  /**
   * A small card that shows while the pointer rests on (or keyboard focus is on) its anchor,
   * on the menu surface so it reads as one family with menus. It sits under the anchor,
   * right-aligned to it, and flips above it near the window's bottom.
   */
  interface Props { label: string; children: Snippet; card: Snippet }
  let { label, children, card }: Props = $props();

  const DELAY_MS = 300;
  const id = `hc-${Math.random().toString(36).slice(2, 8)}`;
  let anchor = $state<HTMLElement>();
  let at = $state<{ right: number; top: number; bottom: number } | null>(null);
  let h = $state(0);
  let w = $state(0);
  let timer: ReturnType<typeof setTimeout> | undefined;

  function show(now = false) {
    clearTimeout(timer);
    const open = () => {
      const r = anchor?.getBoundingClientRect();
      if (r) at = { right: r.right, top: r.top, bottom: r.bottom };
    };
    if (now) open();
    else timer = setTimeout(open, DELAY_MS);
  }
  function hide() {
    clearTimeout(timer);
    at = null;
  }
  $effect(() => () => clearTimeout(timer));

  const left = $derived(at ? Math.max(8, Math.min(at.right - w, window.innerWidth - w - 8)) : 0);
  const top = $derived(at ? (at.bottom + 6 + h > window.innerHeight - 8 ? Math.max(8, at.top - 6 - h) : at.bottom + 6) : 0);
</script>

<svelte:window onscroll={hide} onresize={hide} onkeydown={(e) => e.key === 'Escape' && hide()} />
<!-- Focusable so keyboards can show the card too; it describes, it does nothing. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_static_element_interactions -->
<span class="hc-anchor" bind:this={anchor} tabindex="0" aria-describedby={at ? id : undefined} aria-label={label}
  onpointerenter={() => show()} onpointerleave={hide} onfocus={() => show(true)} onblur={hide}>{@render children()}</span>
{#if at}
  <div class="menu hc" role="tooltip" {id} bind:offsetHeight={h} bind:offsetWidth={w} style="left: {left}px; top: {top}px">{@render card()}</div>
{/if}

<style>
  .hc-anchor{cursor:default;text-decoration:underline dotted color-mix(in srgb,currentColor 45%,transparent);text-underline-offset:3px;border-radius:3px}
  .hc{position:fixed;z-index:50;min-width:160px;max-width:260px;padding:6px 0;pointer-events:none;font-family:var(--font);font-size:13px;white-space:normal;color:var(--text);text-align:left}
</style>

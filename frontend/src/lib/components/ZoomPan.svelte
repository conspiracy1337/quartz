<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from '../ui/Icon.svelte';
  import { FIT, MAX_ZOOM, ZOOM_STEP, clampView, fitted, pan, zoomAt, type Size, type View } from '../zoom';

  /**
   * A picture that fits its box and zooms toward the cursor on the wheel; drag pans it
   * once zoomed. A click that didn't drag calls onclick; without one a double-click zooms
   * in there (or back out). The zoom bar sits bottom right, tools appended to it; with step
   * the edges get previous and next arrows.
   */
  interface Props {
    src?: string | undefined;
    /** Shown instead of a picture that has no image (preview art). */
    art?: string | undefined;
    /** The picture's pixel size, known before it loads; else read from the loaded image (16:9 until then). */
    size?: Size | undefined;
    label: string;
    view?: View;
    onclick?: (() => void) | undefined;
    step?: ((by: -1 | 1) => void) | undefined;
    tools?: Snippet;
  }
  let { src, art, size, label, view = $bindable(FIT), onclick, step, tools }: Props = $props();

  let el: HTMLDivElement;
  let w = $state(0), h = $state(0);
  let loaded = $state<Size>({ w: 16, h: 9 });
  const nat = $derived(size?.w && size.h ? size : loaded);
  const box = $derived({ w, h });
  const v = $derived(clampView(view, box, nat));
  const fit = $derived(fitted(nat, box));
  // Past the picture's own pixels, they show as blocks rather than a blur.
  const pixelated = $derived(src && fit.w * v.s > nat.w);

  // Another picture starts fitted.
  $effect(() => {
    void src;
    void art;
    view = FIT;
  });

  // The wheel is the zoom here, so the page under it must not scroll (Svelte's handler is passive).
  $effect(() => {
    const wheel = (e: WheelEvent) => {
      e.preventDefault();
      const r = el.getBoundingClientRect();
      view = zoomAt(v, Math.exp(-e.deltaY * (e.deltaMode ? 0.05 : 0.002)), e.clientX - r.left, e.clientY - r.top, box, nat);
    };
    el.addEventListener('wheel', wheel, { passive: false });
    return () => el.removeEventListener('wheel', wheel);
  });

  /** Zooms about the centre: by ZOOM_STEP for +1, back for -1, to fit for 0. */
  export function zoom(dir: -1 | 0 | 1) {
    view = dir ? zoomAt(v, dir > 0 ? ZOOM_STEP : 1 / ZOOM_STEP, w / 2, h / 2, box, nat) : FIT;
  }
  /** The zoom keys (+, -, 0); true when it was one. */
  export function key(e: KeyboardEvent): boolean {
    const dir = ({ '+': 1, '=': 1, '-': -1, '0': 0 } as const)[e.key];
    if (dir === undefined || e.ctrlKey || e.metaKey || e.altKey) return false;
    zoom(dir);
    return true;
  }
  // The zoom as a share of the picture's own pixels.
  const percent = $derived(nat.w ? Math.round((fit.w * v.s * 100) / nat.w) : 100);

  // Pointer capture keeps a pan going outside the box until the button is let go.
  let drag = $state<{ x: number; y: number; v: View; moved: boolean } | null>(null);
  function down(e: PointerEvent) {
    if (e.button !== 0 || (e.target as Element).closest('button, .zp-bar')) return;
    el.setPointerCapture(e.pointerId);
    drag = { x: e.clientX, y: e.clientY, v, moved: false };
  }
  function move(e: PointerEvent) {
    if (!drag) return;
    const dx = e.clientX - drag.x, dy = e.clientY - drag.y;
    if (!drag.moved && Math.hypot(dx, dy) < 4) return;
    drag.moved = true;
    view = pan(drag.v, dx, dy, box, nat);
  }
  function up(e: PointerEvent) {
    const click = drag && !drag.moved && e.type === 'pointerup';
    drag = null;
    if (click) onclick?.();
  }
  function load(e: Event) {
    const im = e.currentTarget as HTMLImageElement;
    loaded = { w: im.naturalWidth, h: im.naturalHeight };
  }
  function dbl(e: MouseEvent) {
    if (onclick || (e.target as Element).closest('button, .zp-bar')) return;
    const r = el.getBoundingClientRect();
    view = v.s > 1 ? FIT : zoomAt(v, 2.5, e.clientX - r.left, e.clientY - r.top, box, nat);
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="zp" class:zoomed={v.s > 1} class:open={!!onclick} class:drag={drag?.moved} bind:this={el} bind:clientWidth={w} bind:clientHeight={h}
  onpointerdown={down} onpointermove={move} onpointerup={up} onpointercancel={up} ondblclick={dbl}>
  {#if fit.w}
    <div class="zp-pic" role="img" aria-label={label} style="width: {fit.w}px; height: {fit.h}px; transform: translate(-50%,-50%) translate({v.x}px,{v.y}px) scale({v.s}); {src ? '' : `background: ${art ?? 'var(--console)'}`}">
      {#if src}<img {src} alt="" draggable="false" class:px={pixelated} onload={load} />{/if}
    </div>
  {/if}
  {#if step}
    <button class="btn btn-icon zp-edge" style="left: 12px" onclick={() => step(-1)} aria-label="Previous"><Icon name="chevron-left" size={14} sw={2} /></button>
    <button class="btn btn-icon zp-edge" style="right: 12px" onclick={() => step(1)} aria-label="Next"><Icon name="chevron-right" size={14} sw={2} /></button>
  {/if}
  <div class="zp-bar" role="toolbar" aria-label="Zoom">
    <button class="btn btn-sm btn-icon btn-ghost" disabled={v.s <= 1} onclick={() => zoom(-1)} aria-label="Zoom out"><Icon name="minus" size={14} sw={2} /></button>
    <button class="btn btn-sm btn-ghost tnum zp-pct" disabled={v.s <= 1} onclick={() => zoom(0)} title="Fit">{percent}%</button>
    <button class="btn btn-sm btn-icon btn-ghost" disabled={v.s >= MAX_ZOOM} onclick={() => zoom(1)} aria-label="Zoom in"><Icon name="plus" size={14} sw={2} /></button>
    {@render tools?.()}
  </div>
</div>

<style>
  .zp{position:relative;width:100%;height:100%;overflow:hidden;min-width:0;min-height:0;touch-action:none;user-select:none;cursor:zoom-in}
  .zp.open{cursor:pointer}
  .zp.zoomed{cursor:grab}
  .zp.drag{cursor:grabbing}
  .zp-pic{position:absolute;left:50%;top:50%;transform-origin:center}
  .zp-pic img{display:block;width:100%;height:100%}
  .zp-pic img.px{image-rendering:pixelated}
  /* The skin preview's control bar. */
  .zp-edge{position:absolute;top:50%;margin-top:-16px;background:color-mix(in srgb,var(--panel) 80%,transparent);border-color:var(--line);cursor:default}
  .zp-edge:hover{background:var(--raise)}
  .zp-bar{position:absolute;right:12px;bottom:12px;display:flex;gap:6px;padding:4px;border-radius:8px;background:color-mix(in srgb,var(--panel) 80%,transparent);border:1px solid var(--line);cursor:default}
  .zp-pct{width:56px;padding:0}
</style>

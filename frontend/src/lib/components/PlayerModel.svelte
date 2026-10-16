<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from '../ui/Icon.svelte';
  import { SkinScene } from './skin3d';
  import SkinFigure from './SkinFigure.svelte';

  /**
   * The player in 3D (WebGL, see skin3d). `texture` is a skin PNG URL, `capeTexture` a cape PNG
   * URL (leave it out for none). It draws only when something changes, and animates only while
   * walking or idling on screen; without WebGL it shows the flat front view.
   */
  interface Props {
    texture: string;
    capeTexture?: string | undefined;
    slim?: boolean;
    scale?: number;
    controls?: boolean;
    fill?: boolean;
  }
  let { texture, capeTexture, slim = false, scale: s = 9, controls = true, fill = false }: Props = $props();

  const FRONT = { ry: -28, rx: -10 };
  const DEG = Math.PI / 180;
  // One walk step is 0.6 s each way, the arms and legs swinging 28°.
  const STEP = 0.6, SWING = 28 * DEG, CAPE = 10.8 * DEG;
  let ry = $state(FRONT.ry);
  let rx = $state(FRONT.rx);
  let walking = $state(false);
  let drag = $state<{ x: number; y: number; ry: number; rx: number } | null>(null);
  let flat = $state(false);

  let canvas: HTMLCanvasElement;
  let scene: SkinScene | null = null;
  let raf = 0;
  let onScreen = true;
  let still = false;
  const t0 = performance.now();

  /** Idle sway runs where the model can be handled (the Skins page); Home's card stays put. */
  const moving = () => !still && onScreen && (walking || controls);

  function frame(now: number) {
    raf = 0;
    if (!scene) return;
    const t = (now - t0) / 1000;
    const phase = still ? 0 : Math.sin((t * Math.PI) / STEP);
    const swing = walking ? SWING * phase : 0;
    const sway = controls && !still && !walking ? 0.03 + 0.025 * Math.cos(t * 1.4) : 0;
    const cape = walking ? 2 * CAPE + 0.06 * Math.abs(phase) : CAPE + (sway ? 0.02 * Math.sin(t * 1.1) : 0);
    scene.draw({ yaw: ry * DEG, pitch: -rx * DEG, swing, sway, cape }, s);
    if (moving()) raf = requestAnimationFrame(frame);
  }
  const kick = () => { if (!raf) raf = requestAnimationFrame(frame); };

  onMount(() => {
    scene = new SkinScene(canvas, kick);
    if (!scene.ok) {
      flat = true;
      scene = null;
      return;
    }
    const root = document.documentElement;
    const media = window.matchMedia('(prefers-reduced-motion: reduce)');
    const motion = () => {
      still = root.dataset.motion === 'reduce' || (root.dataset.motion !== 'full' && media.matches);
      kick();
    };
    motion();
    media.addEventListener('change', motion);
    const mo = new MutationObserver(motion);
    mo.observe(root, { attributes: true, attributeFilter: ['data-motion'] });
    const ro = new ResizeObserver(kick);
    ro.observe(canvas);
    const io = new IntersectionObserver((e) => { onScreen = e.some((x) => x.isIntersecting); kick(); });
    io.observe(canvas);
    const lost = (e: Event) => e.preventDefault();
    const restored = () => { scene?.restore(); scene?.setSkin(texture, slim); scene?.setCape(capeTexture); kick(); };
    canvas.addEventListener('webglcontextlost', lost);
    canvas.addEventListener('webglcontextrestored', restored);
    return () => {
      cancelAnimationFrame(raf);
      media.removeEventListener('change', motion);
      mo.disconnect();
      ro.disconnect();
      io.disconnect();
      canvas.removeEventListener('webglcontextlost', lost);
      canvas.removeEventListener('webglcontextrestored', restored);
      scene?.destroy();
      scene = null;
    };
  });

  // Anything the picture depends on draws it again.
  $effect(() => {
    scene?.setSkin(texture, slim);
    scene?.setCape(capeTexture);
    void [ry, rx, s, walking, controls];
    kick();
  });

  // The pointer is captured, so a drag keeps turning the model outside its box until the button is let go.
  function down(e: PointerEvent) {
    if (e.button !== 0) return;
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    drag = { x: e.clientX, y: e.clientY, ry, rx };
  }
  function move(e: PointerEvent) {
    if (!drag) return;
    ry = drag.ry + (e.clientX - drag.x) * 0.6;
    rx = Math.max(-40, Math.min(30, drag.rx - (e.clientY - drag.y) * 0.3));
  }
  const up = () => (drag = null);
  const turn = (d: number) => (ry += d);
  const face = (y: number) => ((ry = y), (rx = FRONT.rx));
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="pm" class:fill class:drag={!!drag} style="font-family: var(--font); color: var(--text)"
  onpointerdown={down} onpointermove={move} onpointerup={up} onpointercancel={up} onlostpointercapture={up}>
  <div class="pm-gl" role="img" aria-label="3D preview of the player skin. Drag to rotate.">
    <canvas bind:this={canvas} class:hide={flat}></canvas>
    {#if flat}<SkinFigure look={{ texture, slim }} zoom={s} />{/if}
  </div>
  {#if controls}
    <div class="pm-ctrl" onpointerdown={(e) => e.stopPropagation()} role="toolbar" tabindex="-1" aria-label="Preview controls">
      <button class="btn btn-sm btn-icon btn-ghost" onclick={() => turn(-45)} aria-label="Rotate left"><Icon name="rotate-left" size={14} sw={2} /></button>
      <button class="btn btn-sm btn-ghost" onclick={() => (walking = !walking)} aria-pressed={walking}>Walk</button>
      <button class="btn btn-sm btn-ghost" onclick={() => face(FRONT.ry)}>Front</button>
      <button class="btn btn-sm btn-ghost" onclick={() => face(152)}>Back</button>
      <button class="btn btn-sm btn-icon btn-ghost" onclick={() => turn(45)} aria-label="Rotate right"><Icon name="rotate-right" size={14} sw={2} /></button>
    </div>
  {/if}
</div>

<style>
  .pm{position:relative;width:100%;height:100%;min-height:320px;display:grid;place-items:center;touch-action:none;cursor:grab;user-select:none;overflow:hidden}
  .pm.drag{cursor:grabbing}
  .pm.fill{position:absolute;inset:0;width:auto;height:auto;min-height:0}
  .pm-gl{position:absolute;inset:0;display:grid;place-items:center}
  .pm-gl canvas{position:absolute;inset:0;width:100%;height:100%}
  .pm-gl canvas.hide{display:none}
  .pm-ctrl{position:absolute;left:50%;bottom:12px;transform:translateX(-50%);display:flex;gap:6px;padding:4px;border-radius:8px;background:color-mix(in srgb,var(--panel) 80%,transparent);border:1px solid var(--line)}
</style>

<script lang="ts">
  import type { SkinColors } from '../api/types';
  import Icon from '../ui/Icon.svelte';
  import { STEVE, uvBox, type Size, type UV } from './skinUV';

  /**
   * CSS-3D player built from boxes. With `texture` (a skin PNG URL) every face is cut from the
   * skin, overlays included; without it the flat `colors` drawing is used. `cape` is a cape
   * background for the colour drawing, `capeTexture` a cape PNG URL; leave both out for none.
   */
  interface Props {
    colors?: SkinColors | undefined;
    texture?: string | undefined;
    cape?: string | undefined;
    capeTexture?: string | undefined;
    slim?: boolean;
    walk?: boolean;
    scale?: number;
    controls?: boolean;
    fill?: boolean;
  }
  let { colors: k = STEVE, texture, cape, capeTexture, slim = false, walk = false, scale: s = 9, controls = true, fill = false }: Props = $props();

  const FRONT = { ry: -28, rx: -10 };
  let ry = $state(FRONT.ry);
  let rx = $state(FRONT.rx);
  let walkSet = $state<boolean | null>(null);
  let drag = $state<{ x: number; y: number; ry: number; rx: number } | null>(null);
  const walking = $derived(walkSet ?? walk);

  interface Face { w: number; h: number; ml: number; mt: number; tf: string; bg: string; br: number; head: boolean }
  interface Colors { front: string; side: string; back?: string; top: string; bottom?: string; head?: boolean }
  const split = (a: string, b: string, p: number) => `linear-gradient(to bottom,${a} 0 ${p}%,${b} ${p}% 100%)`;

  function box(w: number, h: number, d: number, c: Colors): Face[] {
    const W = w * s, H = h * s, D = d * s, oy = H / 2;
    const f = (fw: number, fh: number, tf: string, bg: string, br: number, head = false): Face => ({ w: fw, h: fh, ml: -fw / 2, mt: -fh / 2, tf, bg, br, head });
    return [
      f(W, H, `translateY(${oy}px) translateZ(${D / 2}px)`, c.front, 1, !!c.head),
      f(W, H, `translateY(${oy}px) rotateY(180deg) translateZ(${D / 2}px)`, c.back ?? c.side, 0.72),
      f(D, H, `translateY(${oy}px) rotateY(90deg) translateZ(${W / 2}px)`, c.side, 0.86),
      f(D, H, `translateY(${oy}px) rotateY(-90deg) translateZ(${W / 2}px)`, c.side, 0.86),
      f(W, D, 'rotateX(90deg)', c.top, 1.12),
      f(W, D, `translateY(${H}px) rotateX(-90deg)`, c.bottom ?? c.side, 0.6),
    ];
  }

  const part = (x: number, yTop: number, z: number, rot: string, faces: Face[]) => ({ x: x * s, y: yTop * s, z: z * s, rot, faces });

  /** Skin-textured faces of a w×h×d box at texture offset uv, grown by `inflate` texels. */
  function tex(url: string, w: number, h: number, d: number, uv: UV, inflate = 0, size: Size = [64, 64]): Face[] {
    const geo = box(w + inflate, h + inflate, d + inflate, { front: '', side: '', top: '' });
    return uvBox(url, [w, h, d], uv, size, s, inflate).map((bg, i) => ({ ...geo[i]!, bg }));
  }

  const textured = $derived.by(() => {
    if (!texture) return null;
    const t = texture;
    const aw = slim ? 3 : 4;
    // Each part is the base layer plus the overlay layer, grown by `grow` texels and centred on it.
    const layer = (x: number, y: number, rot: string, w: number, h: number, d: number, base: UV, over: UV, grow: number) => [
      part(x, y, 0, rot, tex(t, w, h, d, base)),
      part(x, y - grow / 2, 0, rot, tex(t, w, h, d, over, grow)),
    ];
    const list = [
      ...layer(0, -14, '', 8, 8, 8, [0, 0], [32, 0], 1),
      ...layer(0, -6, '', 8, 12, 4, [16, 16], [16, 32], 0.5),
      ...layer(-(4 + aw / 2), -6, 'swing-a', aw, 12, 4, [40, 16], [40, 32], 0.5),
      ...layer(4 + aw / 2, -6, 'swing-b', aw, 12, 4, [32, 48], [48, 48], 0.5),
      ...layer(-2, 6, 'swing-b', 4, 12, 4, [0, 16], [0, 32], 0.5),
      ...layer(2, 6, 'swing-a', 4, 12, 4, [16, 48], [0, 48], 0.5),
    ];
    if (capeTexture) {
      // Minecraft's cape box faces backwards: its "front" texture is the outer side.
      const faces = tex(capeTexture, 10, 16, 1, [0, 0], 0, [64, 32]);
      [faces[0], faces[1]] = [{ ...faces[0]!, bg: faces[1]!.bg }, { ...faces[1]!, bg: faces[0]!.bg }];
      [faces[2], faces[3]] = [{ ...faces[2]!, bg: faces[3]!.bg }, { ...faces[3]!, bg: faces[2]!.bg }];
      list.push(part(0, -6, -2.5, 'cape-tilt', faces));
    }
    return list;
  });

  const parts = $derived.by(() => {
    if (textured) return textured;
    const aw = slim ? 3 : 4;
    const sleeve = split(k.top, k.skin, 33);
    const leg = split(k.pants, k.shoes, 75);
    const list = [
      part(0, -14, 0, '', box(8, 8, 8, { front: k.skin, side: split(k.hair, k.skin, 30), back: k.hair, top: k.hair, bottom: k.skin, head: true })),
      part(0, -6, 0, '', box(8, 12, 4, { front: `linear-gradient(90deg,${k.top} 0 47%,${k.topD} 47% 53%,${k.top} 53% 100%)`, side: k.top, back: k.topD, top: k.top, bottom: k.pants })),
      part(-(4 + aw / 2), -6, 0, 'swing-a', box(aw, 12, 4, { front: sleeve, side: sleeve, top: k.top, bottom: k.skin })),
      part(4 + aw / 2, -6, 0, 'swing-b', box(aw, 12, 4, { front: sleeve, side: sleeve, top: k.top, bottom: k.skin })),
      part(-2, 6, 0, 'swing-b', box(4, 12, 4, { front: leg, side: leg, top: k.pants, bottom: k.shoes })),
      part(2, 6, 0, 'swing-a', box(4, 12, 4, { front: leg, side: leg, top: k.pants, bottom: k.shoes })),
    ];
    if (cape) list.push(part(0, -6, -2.5, 'cape-tilt', box(10, 16, 1, { front: '#2a2a30', back: cape, side: '#3a3a40', top: '#3a3a40' })));
    return list;
  });

  function down(e: PointerEvent) {
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
<div class="pm" class:fill class:walk={walking} class:drag={!!drag} style="font-family: var(--font); color: var(--text)"
  onpointerdown={down} onpointermove={move} onpointerup={up} onpointerleave={up}>
  <div class="pm-stage" role="img" aria-label="3D preview of the player skin. Drag to rotate." style="transform: translateY({-2 * s}px) rotateX({rx}deg) rotateY({ry}deg)">
    <div class="pm-floor" style="width: {18 * s}px; height: {10 * s}px; margin-left: {-9 * s}px; margin-top: {-5 * s}px; transform: translateY({18 * s}px) rotateX(90deg)"></div>
    {#each parts as p, pi (pi)}
      <div class="pm-part" style="transform: translate3d({p.x}px, {p.y}px, {p.z}px)">
        <div class="pm-rot {p.rot}">
          {#each p.faces as f, fi (fi)}
            <div class="pm-face" style="width: {f.w}px; height: {f.h}px; margin-left: {f.ml}px; margin-top: {f.mt}px; transform: {f.tf}; background: {f.bg}; filter: brightness({f.br})">
              {#if f.head}
                <span class="pm-fringe" style="left: 0; top: 0; width: 100%; height: 25%; background: {k.hair}"></span>
                <span class="pm-fringe" style="left: 0; top: 25%; width: 12.5%; height: 12.5%; background: {k.hair}"></span>
                <span class="pm-fringe" style="right: 0; top: 25%; width: 12.5%; height: 12.5%; background: {k.hair}"></span>
                <span class="pm-eye" style="left: 12.5%; top: 50%; width: 25%; height: 12.5%; background: #f1f1f1"></span>
                <span class="pm-iris" style="left: 25%; top: 50%; width: 12.5%; height: 12.5%; background: {k.eye}"></span>
                <span class="pm-eye" style="left: 62.5%; top: 50%; width: 25%; height: 12.5%; background: #f1f1f1"></span>
                <span class="pm-iris" style="left: 62.5%; top: 50%; width: 12.5%; height: 12.5%; background: {k.eye}"></span>
                <span class="pm-mouth" style="left: 37.5%; top: 75%; width: 25%; height: 12.5%; background: {k.mouth}"></span>
              {/if}
            </div>
          {/each}
        </div>
      </div>
    {/each}
  </div>
  {#if controls}
    <div class="pm-ctrl" onpointerdown={(e) => e.stopPropagation()} role="toolbar" tabindex="-1" aria-label="Preview controls">
      <button class="btn btn-sm btn-icon btn-ghost" onclick={() => turn(-45)} aria-label="Rotate left"><Icon name="rotate-left" size={14} sw={2} /></button>
      <button class="btn btn-sm btn-ghost" onclick={() => (walkSet = !walking)} aria-pressed={walking}>{walking ? 'Stop' : 'Walk'}</button>
      <button class="btn btn-sm btn-ghost" onclick={() => face(FRONT.ry)}>Front</button>
      <button class="btn btn-sm btn-ghost" onclick={() => face(152)}>Back</button>
      <button class="btn btn-sm btn-icon btn-ghost" onclick={() => turn(45)} aria-label="Rotate right"><Icon name="rotate-right" size={14} sw={2} /></button>
    </div>
  {/if}
</div>

<style>
  .pm{position:relative;width:100%;height:100%;min-height:320px;display:grid;place-items:center;perspective:1100px;touch-action:none;cursor:grab;user-select:none;overflow:hidden}
  .pm.drag{cursor:grabbing}
  .pm.fill{position:absolute;inset:0;width:auto;height:auto;min-height:0}
  .pm-stage{position:relative;width:0;height:0;transform-style:preserve-3d}
  .pm-part{position:absolute;left:0;top:0;transform-style:preserve-3d}
  .pm-rot{position:absolute;left:0;top:0;transform-style:preserve-3d}
  .pm-face{position:absolute;left:0;top:0;backface-visibility:hidden;image-rendering:pixelated}
  .pm-floor{position:absolute;left:0;top:0;border-radius:50%;background:radial-gradient(closest-side,rgba(0,0,0,.55),transparent);transform-style:preserve-3d}
  @media (prefers-reduced-motion:no-preference){
    .walk :global(.swing-a){animation:swa 900ms ease-in-out infinite alternate}
    .walk :global(.swing-b){animation:swb 900ms ease-in-out infinite alternate}
  }
  @keyframes swa{from{transform:rotateX(28deg)}to{transform:rotateX(-28deg)}}
  @keyframes swb{from{transform:rotateX(-28deg)}to{transform:rotateX(28deg)}}
  .pm :global(.cape-tilt){transform:rotateX(-10deg)}
  .walk :global(.cape-tilt){transform:rotateX(-22deg)}
  .pm-eye,.pm-iris,.pm-mouth,.pm-fringe{position:absolute}
  .pm-ctrl{position:absolute;left:50%;bottom:12px;transform:translateX(-50%);display:flex;gap:6px;padding:4px;border-radius:8px;background:color-mix(in srgb,var(--panel) 80%,transparent);border:1px solid var(--line)}
</style>

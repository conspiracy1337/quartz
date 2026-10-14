<script lang="ts">
  import { figure, type SkinLook } from '../look';

  /** The flat front view of a skin, `zoom` pixels per texel (skin tiles, and PlayerModel without WebGL). */
  interface Props { look: SkinLook; zoom?: number }
  let { look, zoom = 3.4 }: Props = $props();
  const f = $derived(figure(look));
</script>

<span class="fig" aria-hidden="true" style="grid-template-columns: {f.arm}px 8px {f.arm}px; width: {8 + 2 * f.arm}px; transform: scale({zoom})"><span class="h" style="background: {f.h}"></span><span class="al" style="background: {f.al}"></span><span class="b" style="background: {f.b}"></span><span class="ar" style="background: {f.ar}"></span><span class="l"><span style="background: {f.ll}"></span><span style="background: {f.lr}"></span></span></span>

<style>
  .fig{display:grid;grid-template-rows:8px 12px 12px;image-rendering:pixelated;margin:auto}
  .fig .h{grid-column:2;grid-row:1}.fig .b{grid-column:2;grid-row:2}.fig .al{grid-column:1;grid-row:2}.fig .ar{grid-column:3;grid-row:2}.fig .l{grid-column:2;grid-row:3;display:grid;grid-template-columns:1fr 1fr}
</style>

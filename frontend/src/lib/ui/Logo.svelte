<script lang="ts">
  // The app's mark: the 14×14 quartz texture (assets/texture.png), stored at 12×
  // with nearest-neighbour scaling. It's only drawn at whole multiples of its
  // 14 texels, so every texel stays square on 1× and 2× screens. Decorative; the product name always
  // sits beside it.
  import logo from '$assets/logo.png';

  let { scale = 2 }: { scale?: 1 | 2 | 3 } = $props();
</script>

<img src={logo} width={14 * scale} height={14 * scale} alt="" draggable="false" style="image-rendering: pixelated; flex: none" />

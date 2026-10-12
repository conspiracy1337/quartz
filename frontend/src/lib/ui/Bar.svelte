<script lang="ts">
  /** The one progress bar style: 8px, 2px corners, square-edged fill. */
  interface Props { pct: number; label?: string; color?: string; live?: boolean; style?: string }
  let { pct, label, color, live = false, style }: Props = $props();
</script>

<span class="bar" class:live {style} role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100} aria-label={label}>
  <span style="width: {pct}%{color ? `; background: ${color}` : ''}"></span>
</span>

<style>
  .bar{display:block}
  @media (prefers-reduced-motion:no-preference){.bar.live > span{background-image:linear-gradient(90deg,transparent,rgba(255,255,255,.25),transparent);background-size:40% 100%;background-repeat:no-repeat;animation:sweep 1.4s linear infinite}}
  @keyframes sweep{from{background-position:-40% 0}to{background-position:140% 0}}
</style>

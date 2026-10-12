<script lang="ts">
  /** A row of round colour swatches (Appearance's accent picker; also instance tile colours). */
  interface Props { colors: readonly string[]; value: string; label: string; onpick: (c: string) => void }
  let { colors, value, label, onpick }: Props = $props();
</script>

<div style="display: flex; gap: 10px; align-items: center; flex-wrap: wrap" role="radiogroup" aria-label={label}>
  {#each colors as c (c)}<button class="acc" class:on={value.toLowerCase() === c} style="background: {c}" onclick={() => onpick(c)} role="radio" aria-checked={value.toLowerCase() === c} aria-label={c}></button>{/each}
</div>

<style>
  .acc{width:28px;height:28px;border-radius:50%;border:2px solid transparent;padding:0}
  .acc.on{border-color:var(--text);box-shadow:inset 0 0 0 2px var(--bg)}
</style>

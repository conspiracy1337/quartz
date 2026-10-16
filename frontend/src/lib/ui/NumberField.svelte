<script lang="ts">
  import Icon from './Icon.svelte';

  /** A whole-number field with a title, an optional unit, and up/down steppers at its end
      that move the value by one within min and max. Typing stays free; the caller checks. */
  interface Props { label: string; value: string; min: number; max: number; unit?: string; mono?: boolean }
  let { label, value = $bindable(), min, max, unit = '', mono = false }: Props = $props();

  function step(d: number) {
    const n = /^\d+$/.test(value.trim()) ? Number(value.trim()) : min;
    value = String(Math.min(max, Math.max(min, n + d)));
  }
</script>

<label class="field"><span class="field-label trunc">{label}</span>
  <span class="input nf">
    <input class={mono ? 'mono' : 'tnum'} bind:value inputmode="numeric" onkeydown={(e) => { if (e.key === 'ArrowUp' || e.key === 'ArrowDown') { e.preventDefault(); step(e.key === 'ArrowUp' ? 1 : -1); } }} />
    {#if unit}<span class="suffix">{unit}</span>{/if}
    <span class="nf-step"><button type="button" tabindex="-1" aria-label="Increase {label}" onclick={() => step(1)}><Icon name="chevron-up" size={10} sw={2.6} /></button><button type="button" tabindex="-1" aria-label="Decrease {label}" onclick={() => step(-1)}><Icon name="chevron-down" size={10} sw={2.6} /></button></span>
  </span>
</label>

<style>
  .nf{padding-right:2px}
  .nf-step{display:flex;flex-direction:column;margin-left:auto}
  .nf-step button{height:13px;width:18px;padding:0;border:0;background:none;color:var(--faint);display:grid;place-items:center;border-radius:3px}
  .nf-step button:hover{color:var(--text);background:var(--hover)}
</style>

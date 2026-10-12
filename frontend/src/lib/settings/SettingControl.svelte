<script lang="ts">
  import { prefs } from '../prefs.svelte';
  import { href } from '../router.svelte';
  import Select from '../ui/Select.svelte';
  import Seg from '../ui/Seg.svelte';
  import Toggle from '../ui/Toggle.svelte';
  import type { Control } from './registry';

  /** The control for one registry setting, bound to the live settings. */
  interface Props { control: Control; label: string }
  let { control: c, label }: Props = $props();
  const s = $derived(prefs.value);
  const set = <V,>(fn: (st: NonNullable<typeof s>, v: V) => void) => (v: V) => prefs.update((st) => fn(st, v));
</script>

{#if s}
  {#if c.kind === 'toggle'}
    <Toggle on={c.get(s)} {label} onchange={set(c.set)} />
  {:else if c.kind === 'seg'}
    <Seg options={c.options} value={c.get(s)} {label} onchange={set(c.set)} />
  {:else if c.kind === 'select'}
    <Select options={c.options} value={c.get(s)} {label} onchange={set(c.set)} />
  {:else if c.kind === 'range'}
    {@const v = c.get(s)}
    <span style="display: flex; gap: 12px; align-items: center"><input type="range" class="range" min={c.min} max={c.max} value={v} aria-label={label} oninput={(e) => set(c.set)(Number(e.currentTarget.value))} /><span class="mono" style="width: 20px">{v}</span></span>
  {:else if c.kind === 'memory'}
    <span style="display: grid; grid-template-columns: 1fr 1fr; gap: 8px">
      <label class="input"><span class="sr">Minimum</span><input class="tnum" inputmode="numeric" value={s.java.memoryMinMb} onchange={(e) => prefs.update((st) => (st.java.memoryMinMb = Number(e.currentTarget.value) || 0))} /><span class="suffix">MB min</span></label>
      <label class="input"><span class="sr">Maximum</span><input class="tnum" inputmode="numeric" value={s.java.memoryMaxMb} onchange={(e) => prefs.update((st) => (st.java.memoryMaxMb = Number(e.currentTarget.value) || 0))} /><span class="suffix">MB max</span></label>
    </span>
  {:else if c.kind === 'link'}
    <a href={href(c.route)} class="btn btn-sm">{c.label}</a>
  {/if}
{/if}

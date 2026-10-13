<script lang="ts">
  import { prefs } from '../prefs.svelte';
  import { href } from '../router.svelte';
  import Select from '../ui/Select.svelte';
  import Seg from '../ui/Seg.svelte';
  import Toggle from '../ui/Toggle.svelte';
  import SecretControl from './SecretControl.svelte';
  import type { AppInfo } from '../api/system';
  import type { Control } from './registry';

  /** The control for one registry setting, bound to the live settings. */
  interface Props { control: Control; label: string; info?: AppInfo | undefined }
  let { control: c, label, info }: Props = $props();
  const s = $derived(prefs.value);
  const set = <V,>(fn: (st: NonNullable<typeof s>, v: V) => void) => (v: V) => prefs.update((st) => fn(st, v));
</script>

{#if s}
  {#if c.kind === 'toggle'}
    <Toggle on={c.get(s)} {label} disabled={c.disabled?.(info) ?? false} onchange={set(c.set)} />
  {:else if c.kind === 'seg'}
    <Seg options={c.options} value={c.get(s)} {label} onchange={set(c.set)} />
  {:else if c.kind === 'select'}
    <Select options={c.options} value={c.get(s)} {label} onchange={set(c.set)} />
  {:else if c.kind === 'range'}
    {@const v = c.get(s)}
    <span style="display: flex; gap: 12px; align-items: center"><input type="range" class="range" min={c.min} max={c.max} value={v} aria-label={label} oninput={(e) => set(c.set)(Number(e.currentTarget.value))} /><span class="mono" style="width: 20px">{v}</span></span>
  {:else if c.kind === 'inputs'}
    <span style="display: grid; grid-template-columns: {c.fields.map((f) => `${f.grow ?? 1}fr`).join(' ')}; gap: 8px">
      {#each c.fields as f (f.label)}
        <label class="input"><span class="sr">{f.label}</span><input class:tnum={f.numeric} inputmode={f.numeric ? 'numeric' : 'text'} placeholder={f.placeholder} value={f.get(s)} onchange={(e) => set(f.set)(e.currentTarget.value)} />{#if f.suffix}<span class="suffix">{f.suffix}</span>{/if}</label>
      {/each}
    </span>
  {:else if c.kind === 'secret'}
    <SecretControl control={c} {label} />
  {:else if c.kind === 'link'}
    <a href={href(c.route)} class="btn btn-sm">{c.label}</a>
  {/if}
{/if}

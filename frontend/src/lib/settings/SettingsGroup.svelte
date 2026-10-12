<script lang="ts">
  import { prefs } from '../prefs.svelte';
  import Setting from '../ui/Setting.svelte';
  import SettingControl from './SettingControl.svelte';
  import { visible, type SettingDef } from './registry';

  /** A heading over a card of setting rows. A textarea setting renders as a full-width block. */
  interface Props { title?: string; defs: SettingDef[]; mark?: string }
  let { title, defs: all, mark }: Props = $props();
  const defs = $derived(all.filter((d) => visible(d, prefs.value)));
</script>

{#snippet rows()}
  {#each defs as d, i (d.id)}
    {#if i > 0}<div class="divider"></div>{/if}
    {#if d.control.kind === 'textarea'}
      {@const c = d.control}
      <div style="padding: 10px 16px 14px; display: flex; flex-direction: column; gap: 6px"><span class="setting-title">{d.title}</span><textarea class="textarea" rows={c.rows} aria-label={d.title} value={prefs.value ? c.get(prefs.value) : ''} onchange={(e) => prefs.update((s) => c.set(s, e.currentTarget.value))}></textarea></div>
    {:else if mark}
      <div class="setting"><span><span class="small faint" style="display: block">{d.group}</span><span class="setting-title" style="display: block">{@render highlighted(d.title)}</span></span><SettingControl control={d.control} label={d.title} /></div>
    {:else}
      <Setting title={d.title} {...(d.desc ? { desc: d.desc } : {})}><SettingControl control={d.control} label={d.title} /></Setting>
    {/if}
  {/each}
{/snippet}

{#snippet highlighted(text: string)}
  {@const at = text.toLowerCase().indexOf((mark ?? '').trim().toLowerCase())}
  {#if at < 0 || !mark?.trim()}{text}{:else}{text.slice(0, at)}<mark>{text.slice(at, at + mark.trim().length)}</mark>{text.slice(at + mark.trim().length)}{/if}
{/snippet}

{#if title}
  <section class="grp" aria-label={title}>
    <h2 class="h3">{title}</h2>
    <div class="card">{@render rows()}</div>
  </section>
{:else}
  <div class="card">{@render rows()}</div>
{/if}

<style>
  mark{background:color-mix(in srgb,var(--accent) 28%,transparent);color:var(--text);border-radius:2px;padding:0 1px}
</style>

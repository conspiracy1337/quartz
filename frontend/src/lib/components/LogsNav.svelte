<script lang="ts">
  import { href } from '../router.svelte';

  /** The Live / Files / History switch shared by the three log views. */
  interface Props { instanceId: string; active: 'live' | 'files' | 'history'; style?: string }
  let { instanceId, active, style }: Props = $props();
  const VIEWS = [{ key: 'live', label: 'Live' }, { key: 'files', label: 'Files' }, { key: 'history', label: 'History' }] as const;
  const to = (k: string) => href({ name: 'instance', id: instanceId, tab: 'logs', ...(k === 'live' ? {} : { sub: k }) });
</script>

<div class="seg" role="tablist" {style}>
  {#each VIEWS as v (v.key)}
    {#if v.key === active}
      <button class="on" role="tab" aria-selected="true">{#if v.key === 'live'}<span class="dot live" aria-hidden="true"></span>{/if}{v.label}</button>
    {:else}
      <a href={to(v.key)} class="btn btn-ghost btn-sm" role="tab" aria-selected="false" style="height: 26px">{v.label}</a>
    {/if}
  {/each}
</div>

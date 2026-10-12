<script lang="ts">
  import { api } from '$lib/api';
  import type { Instance } from '$lib/api/types';
  import { fullDate } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import { go, href } from '$lib/router.svelte';
  import { CHANNELS } from '$lib/settings/registry';
  import { session } from '$lib/session.svelte';
  import Check from '$lib/ui/Check.svelte';
  import Dialog from '$lib/ui/Dialog.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Seg from '$lib/ui/Seg.svelte';
  import Tile from '$lib/ui/Tile.svelte';

  interface Props { instance: Instance }
  let { instance: i }: Props = $props();

  const updates = query(() => api.review.modUpdates(i.id));
  let picked = $state<Record<number, boolean>>({ 0: true, 1: true, 2: true, 3: true });
  let channel = $state<string>('release');
  let open = $state(-1);
  const list = $derived(updates.data ?? []);
  const count = $derived(list.filter((_, k) => picked[k]).length);
  const cur = $derived(list[open]);
  const back = $derived(href({ name: 'instance', id: i.id, tab: 'mods' }));

  function apply() {
    go({ name: 'instance', id: i.id, tab: 'mods' });
  }
</script>

<div class="topbar">
  <a href={back} class="btn btn-sm btn-ghost btn-icon" aria-label="Back"><Icon name="chevron-left" size={14} sw={2} /></a>
  <h1 class="h2">Updates for {i.name}</h1>
  <div style="flex: 1 1 0"></div>
  <span class="small faint">Channel</span><Seg options={CHANNELS} bind:value={channel} label="Update channel" />
</div>
<div class="scroll">
  <div class="page" style="gap: 12px; padding-top: 16px">
    <div class="list">
      {#each list as u, k (u.name)}
        <div class="row ur">
          <Check on={!!picked[k]} label="Update {u.name}" onchange={(v) => (picked[k] = v)} />
          <Tile art={u.art} />
          <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{u.name}</span><span class="trunc small faint" style="display: block">{u.note}</span></span>
          <span class="mono muted opt" style="display: flex; gap: 8px; align-items: center"><span>{u.from}</span><Icon name="arrow-right" size={12} sw={2} /><span style="color: var(--text)">{u.to}</span></span>
          <span style="display: flex; justify-content: flex-end"><button class="btn btn-sm" onclick={() => (open = k)}>Changelog</button></span>
        </div>
      {/each}
    </div>
    <div class="card" style="display: grid; grid-template-columns: minmax(0,1fr) auto; gap: 14px; align-items: center; padding: 12px 16px">
      <span class="small muted">Distant Horizons is locked to 2.3.0-b and was skipped.</span>
      <button class="btn btn-sm btn-ghost">Unlock</button>
    </div>
    <div style="display: flex; justify-content: flex-end; gap: 8px; align-items: center">
      <span class="small faint tnum" style="margin-right: auto">{count} selected, about {(count * 1.3).toFixed(1)} MB</span>
      <a href={back} class="btn">Cancel</a>
      <button class="btn btn-accent" disabled={count === 0} onclick={apply}>Update {count}</button>
    </div>
  </div>
</div>
{#if cur}
  <Dialog title={cur.name} onclose={() => (open = -1)}>
    {#snippet head()}<span><span class="h2" style="display: block">{cur.name}</span><span class="mono small faint">{cur.from} to {cur.to}</span></span>{/snippet}
    {#each cur.versions as v (v.id)}
      <section aria-label={v.id} style="display: flex; flex-direction: column; gap: 6px">
        <div style="display: flex; justify-content: space-between; align-items: baseline"><span class="mono" style="font-weight: 500">{v.id}</span><span class="small faint">{fullDate(v.date)}</span></div>
        <ul class="cl">{#each v.items as t (t)}<li>{t}</li>{/each}</ul>
      </section>
    {/each}
    {#snippet foot()}<a href={href({ name: 'project', id: 'iris-shaders' })} class="btn" style="margin-right: auto">All versions</a><button class="btn" onclick={() => (open = -1)}>Close</button><button class="btn btn-accent" onclick={() => ((picked[open] = true), (open = -1))}>Keep selected</button>{/snippet}
  </Dialog>
{/if}

<style>
  .ur{grid-template-columns:16px 32px minmax(0,1fr) 200px 96px;height:56px}
  @media (max-width:1100px){.ur{grid-template-columns:16px 32px minmax(0,1fr) 96px}.ur .opt{display:none}}
  .cl{margin:0;padding-left:18px;color:var(--muted);line-height:22px}
</style>

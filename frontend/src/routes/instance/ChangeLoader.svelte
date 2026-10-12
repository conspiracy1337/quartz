<script lang="ts">
  import { api } from '$lib/api';
  import type { Instance } from '$lib/api/types';
  import { shortDate } from '$lib/format';
  import { loaderLabel } from '$lib/labels';
  import { query } from '$lib/query.svelte';
  import { go, href } from '$lib/router.svelte';
  import { revs } from '$lib/revs.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';

  interface Props { instance: Instance }
  let { instance: i }: Props = $props();

  const loaders = query(() => api.versions.loaders(i.minecraft));
  let l = $state<number | null>(null);
  let v = $state(0);
  let beta = $state(false);
  const back = $derived(href({ name: 'instance', id: i.id, tab: 'version' }));
  const currentIndex = $derived(loaders.data?.findIndex((x) => x.kind === (i.loader?.kind ?? 'none')) ?? 0);
  const li = $derived(l ?? currentIndex);
  const opt = $derived(loaders.data?.[li]);
  const list = $derived(opt ? [...opt.stable.map((s, k) => ({ ...s, tag: k === 0 ? 'stable' : '' })), ...(beta ? opt.beta.map((s) => ({ ...s, tag: 'beta' })) : [])] : []);
  const vi = $derived(Math.min(v, Math.max(0, list.length - 1)));
  const sel = $derived(list[vi]);
  const selId = $derived(sel?.id ?? '');
  const isCurrent = $derived(i.loader ? opt?.kind === i.loader.kind && selId === i.loader.id : opt?.kind === 'none');
  const action = $derived(isCurrent ? 'Already installed' : list.length ? `Install ${opt?.name} ${sel?.name}` : 'Remove loader');
  let busy = $state(false);

  async function apply() {
    busy = true;
    try {
      await api.instances.setVersion(i.id, i.minecraft, list.length ? (opt?.kind ?? '') : '', list.length ? selId : '');
      revs.bump('instances');
      go({ name: 'instance', id: i.id, tab: 'version' });
    } catch (e) {
      session.fail('Mod loader', e);
    } finally {
      busy = false;
    }
  }
</script>

<div class="scroll">
  <div class="page" style="gap: 14px; padding-top: 16px">
    <div style="display: flex; align-items: center; gap: 8px"><a href={back} class="btn btn-sm btn-ghost"><Icon name="chevron-left" size={12} sw={2} />Back</a><h2 class="h2" style="margin-right: auto">Mod loader for {i.minecraft}</h2><label class="small muted" style="display: flex; gap: 8px; align-items: center"><Toggle bind:on={beta} label="Show beta versions" />Show beta versions</label></div>
    <div class="picker">
      <section aria-label="Loader">
        <div class="plabel"><span>Loader</span></div>
        <div class="list" role="radiogroup" aria-label="Loader">
          {#each loaders.data ?? [] as o, k (o.kind)}
            <button class="prow" class:on={li === k} onclick={() => ((l = k), (v = 0))} role="radio" aria-checked={li === k}><span style="min-width: 0"><span style="display: block; font-weight: 500">{o.name}</span><span class="trunc small faint" style="display: block">{o.note}{o.stable[0] ? `, latest ${o.stable[0].name}` : ''}</span></span>{#if k === currentIndex}<span class="badge">Current</span>{/if}</button>
          {/each}
        </div>
      </section>
      <section aria-label="Loader version">
        <div class="plabel"><span>{opt?.name ?? ''} versions</span><span class="tnum">{list.length}</span></div>
        {#if list.length}
          <div class="list" role="radiogroup" aria-label="Loader version">
            {#each list as x, k (x.id)}
              <button class="prow min" class:on={vi === k} onclick={() => (v = k)} role="radio" aria-checked={vi === k}><span class="radio" class:on={vi === k} aria-hidden="true"></span><span style="display: flex; gap: 8px; align-items: center; min-width: 0"><span class="mono">{x.name}</span>{#if x.tag}<span class="badge" class:badge-accent={x.tag === 'stable'} class:badge-warn={x.tag === 'beta'}>{x.tag === 'stable' ? 'Latest stable' : 'Beta'}</span>{/if}</span><span class="small faint tnum">{x.released ? shortDate(x.released) : ''}</span></button>
            {/each}
          </div>
        {:else}
          <div class="card empty" style="height: 220px"><span>No loader. Mods will be removed from the launch, but stay in the folder.</span></div>
        {/if}
      </section>
    </div>
  </div>
</div>
<div class="pfoot">
  <span class="muted">{i.loader ? loaderLabel(i.loader) : 'No loader'}</span><Icon name="arrow-right" size={12} sw={2} /><span>{list.length ? `${opt?.name} ${sel?.name}` : 'No loader'}</span>
  <span style="margin-left: auto; display: flex; gap: 8px"><a href={back} class="btn">Cancel</a><button class="btn btn-accent" disabled={isCurrent || busy} onclick={apply}>{action}</button></span>
</div>

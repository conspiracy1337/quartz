<script lang="ts">
  import { api } from '$lib/api';
  import type { Instance } from '$lib/api/types';
  import { shortDate } from '$lib/format';
  import { loaderLabel } from '$lib/labels';
  import { switchVersion } from '$lib/instanceActions';
  import { query } from '$lib/query.svelte';
  import { href } from '$lib/router.svelte';
  import Empty from '$lib/ui/Empty.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import LoaderIcon from '$lib/ui/LoaderIcon.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';

  interface Props { instance: Instance }
  let { instance: i }: Props = $props();

  let tries = $state(0); // Retry after an offline failure
  const loaders = query(() => (void tries, api.versions.loaders(i.minecraft)));
  let l = $state<number | null>(null);
  let v = $state(0);
  let beta = $state(false);
  const back = $derived(href({ name: 'instance', id: i.id, tab: 'settings' }));
  const currentIndex = $derived(loaders.data?.findIndex((x) => x.kind === (i.loader?.kind ?? 'none')) ?? 0);
  const li = $derived(l ?? currentIndex);
  const opt = $derived(loaders.data?.[li]);
  const list = $derived(opt ? [...opt.stable.map((s, k) => ({ ...s, tag: k === 0 ? 'stable' : '' })), ...(beta ? opt.beta.map((s) => ({ ...s, tag: 'beta' })) : [])] : []);
  const vi = $derived(Math.min(v, Math.max(0, list.length - 1)));
  const sel = $derived(list[vi]);
  const selId = $derived(sel?.id ?? '');
  const isCurrent = $derived(i.loader ? opt?.kind === i.loader.kind && selId === i.loader.id : opt?.kind === 'none');
  const action = $derived(!loaders.data ? 'Install' : isCurrent ? 'Already installed' : list.length ? `Install ${opt?.name} ${sel?.name}` : 'Remove loader');
  let busy = $state(false);
  // Moving to another loader: how many mods have a build for it (same wording as Change Minecraft).
  const other = $derived(list.length > 0 && !!i.loader && opt?.kind !== i.loader.kind ? opt?.kind : undefined);
  const check = query(() => (other ? api.instances.compatibility(i.id, i.minecraft, other) : Promise.resolve(null)));
  const compat = $derived(!other ? '' : check.data ? `${check.data.matching} of ${check.data.total} mods have a matching version` : 'Mods will be checked before switching');

  async function apply() {
    busy = true;
    await switchVersion(i, i.minecraft, list.length ? (opt?.kind ?? '') : '', list.length ? selId : '', 'Mod loader');
    busy = false;
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
            <button class="prow ic" class:on={li === k} onclick={() => ((l = k), (v = 0))} role="radio" aria-checked={li === k}><LoaderIcon kind={o.kind} size={18} /><span style="min-width: 0"><span style="display: block; font-weight: 500">{o.name}</span><span class="trunc small faint" style="display: block">{o.note}{o.stable[0] ? `, latest ${o.stable[0].name}` : ''}</span></span>{#if k === currentIndex}<span class="badge">Current</span>{/if}</button>
          {:else}
            {#if loaders.error}<Empty text="Couldn't load versions. Check your connection." action="Retry" onclick={() => tries++} />
            {:else}{#each { length: 5 } as _, k (k)}<div class="prow ic" aria-hidden="true"><span class="sk" style="width: 18px; height: 18px"></span><span style="display: flex; flex-direction: column; gap: 6px"><span class="sk" style="height: 12px; width: {[30, 24, 36, 28, 32][k]}%"></span><span class="sk" style="height: 10px; width: 55%"></span></span></div>{/each}{/if}
          {/each}
        </div>
      </section>
      <section aria-label="Loader version">
        <div class="plabel"><span>{opt ? `${opt.name} versions` : ''}</span><span class="tnum">{loaders.data ? list.length : ''}</span></div>
        {#if !loaders.data}
          <div class="list" aria-hidden="true">{#each { length: 6 } as _, k (k)}<div class="prow min"><span class="sk" style="width: 14px; height: 14px; border-radius: 50%"></span><span class="sk" style="height: 12px; width: {[34, 28, 40, 30, 36, 26][k]}%"></span><span></span></div>{/each}</div>
        {:else if list.length}
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
  <span class="muted">{i.loader ? loaderLabel(i.loader) : 'No loader'}</span><Icon name="arrow-right" size={12} sw={2} /><span>{!loaders.data ? '' : list.length ? `${opt?.name} ${sel?.name}` : 'No loader'}</span>
  {#if compat}<span class="small faint" style="margin-left: 8px">{compat}</span>{/if}
  <span style="margin-left: auto; display: flex; gap: 8px"><a href={back} class="btn">Cancel</a><button class="btn btn-accent" disabled={isCurrent || busy || !loaders.data} onclick={apply}>{action}</button></span>
</div>

<script lang="ts">
  import minecraftNet from '$assets/minecraft-net.png';
  import { api } from '$lib/api';
  import type { Instance } from '$lib/api/types';
  import { bytes, fullDate } from '$lib/format';
  import { LOADER_PRODUCTS } from '$lib/labels';
  import LoaderIcon from '$lib/ui/LoaderIcon.svelte';
  import { lastCount, query, rememberCount } from '$lib/query.svelte';
  import { revs } from '$lib/revs.svelte';
  import { href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';

  // The instance's Minecraft, custom jar and loader, first in its Settings. Java has
  // its own section below ("Java and memory").
  interface Props { instance: Instance }
  let { instance: i }: Props = $props();
  const to = (sub: string) => href({ name: 'instance', id: i.id, tab: 'settings', sub });

  // Keyed, so a second visit shows it at once (Browse shares the version list).
  const families = query(() => api.versions.minecraft(), 'versions.minecraft');
  const loaders = query(() => (i.loader ? api.versions.loaders(i.minecraft) : Promise.resolve([])));

  const family = $derived(families.data?.find((f) => f.releases.some((r) => r.id === i.minecraft) || f.snapshots.some((r) => r.id === i.minecraft)));
  const release = $derived(family && [...family.releases, ...family.snapshots].find((r) => r.id === i.minecraft));
  const mcLine = $derived(release ? `${release.snapshot ? 'Snapshot' : 'Release'}, ${fullDate(release.released)}. Java ${release.java} required.` : '');
  // The newest release in the same family is offered, as the design shows (1.21.4 → 1.21.5).
  // Following the latest release offers the newest of all (the switch is still yours).
  const sameFamily = $derived(family?.releases[0] && family.releases[0].id !== i.minecraft && !release?.snapshot && (family.releases.findIndex((r) => r.id === i.minecraft) > 0) ? family.releases[0] : undefined);
  const newest = $derived.by(() => {
    const all = (families.data ?? []).flatMap((f) => f.releases);
    const at = all.findIndex((r) => r.id === i.minecraft);
    return i.followLatest && !release?.snapshot && at > 0 ? all[0] : undefined;
  });
  const newer = $derived(newest ?? sameFamily);
  const compat = query(() => (newer ? api.instances.compatibility(i.id, newer.id) : Promise.resolve(null)));
  // Whether the newer-release card showed last time, so it holds its place while the versions load.
  // svelte-ignore state_referenced_locally
  const newerKey = `newer.${i.id}`;
  $effect(() => void (families.data && rememberCount(newerKey, newer && !i.clientJar ? 1 : 0)));

  const opt = $derived(loaders.data?.find((o) => o.kind === i.loader?.kind));
  const loaderLine = $derived.by(() => {
    if (!i.loader || !opt) return '';
    const state = opt.stable[0]?.id === i.loader.id ? 'Latest stable.' : opt.stable.some((v) => v.id === i.loader!.id) ? 'Stable build.' : opt.beta.some((v) => v.id === i.loader!.id) ? 'Beta build.' : '';
    return i.loader.kind === 'fabric' || i.loader.kind === 'quilt' ? `${state} Intermediary ${i.minecraft}.`.trim() : state;
  });

  async function edit(fn: () => Promise<unknown>, what: string) {
    try {
      await fn();
      revs.bump('instances');
    } catch (e) {
      session.fail(what, e);
    }
  }
</script>

<!-- A component's second line; a skeleton holds its place until the versions are in. -->
{#snippet line(text: string, ready: boolean)}
  <span class="small faint" style="display: block">{#if ready}{text}{:else}<span class="sk sk-text" style="width: 220px" aria-hidden="true"></span>{/if}</span>
{/snippet}

<section class="sec" aria-label="Version">
  <div class="sec-h"><h2 class="h3">Version</h2></div>
  <div class="card">
    <div class="comp"><span class="cbox" aria-hidden="true"><img src={minecraftNet} width="24" height="24" class="px" alt="" draggable="false" /></span><span><span class="h3" style="display: block">Minecraft {i.minecraft}</span>{@render line(mcLine, !!families.data)}</span>{#if i.clientJar}<button class="btn btn-sm" disabled title="Remove the custom jar to change the version">Change</button>{:else}<a href={to('minecraft')} class="btn btn-sm">Change</a>{/if}</div>
    {#if i.clientJar}
      <div class="divider"></div>
      <div class="comp"><span class="cbox" aria-hidden="true"><Icon name="file" size={20} sw={1.8} /></span><span style="min-width: 0"><span class="h3 trunc" style="display: block">{i.clientJar.name}</span><span class="small faint" style="display: block">Custom jar, {bytes(i.clientJar.size)}. Runs in place of the vanilla game jar.</span></span><button class="btn btn-sm btn-ghost" onclick={() => edit(() => api.instances.removeClientJar(i.id), 'Custom jar')}>Remove</button></div>
    {/if}
    <div class="divider"></div>
    {#if i.loader}
      <div class="comp"><span class="cbox" aria-hidden="true"><LoaderIcon kind={i.loader.kind} size={22} /></span><span><span class="h3" style="display: block">{LOADER_PRODUCTS[i.loader.kind]} {i.loader.version}</span>{@render line(loaderLine, !!loaders.data)}</span><a href={to('loader')} class="btn btn-sm">Change</a></div>
    {:else}
      <div class="comp"><span class="cbox" aria-hidden="true"><LoaderIcon kind="none" size={22} /></span><span><span class="h3" style="display: block">No mod loader</span><span class="small faint" style="display: block">{i.clientJar ? 'Custom jars run without a mod loader.' : 'Vanilla. Add a loader to use mods.'}</span></span>{#if i.clientJar}<button class="btn btn-sm" disabled>Add</button>{:else}<a href={to('loader')} class="btn btn-sm">Add</a>{/if}</div>
    {/if}
    <div class="divider"></div>
    <Setting title="Keep the loader up to date" desc="Installs new stable {i.loader ? LOADER_PRODUCTS[i.loader.kind] : 'loader'} versions when you launch."><Toggle on={i.keepLoaderUpdated} label="Keep the loader up to date" onchange={(v) => edit(() => api.instances.setVersionOptions(i.id, v, i.followLatest), 'Version settings')} /></Setting>
    <div class="divider"></div>
    <Setting title="Follow the latest Minecraft release" desc={i.clientJar ? 'A custom jar stays on the version it was built for.' : 'Moves to new releases once every mod has a matching version.'}><Toggle on={i.followLatest} disabled={!!i.clientJar} label="Follow the latest Minecraft release" onchange={(v) => edit(() => api.instances.setVersionOptions(i.id, i.keepLoaderUpdated, v), 'Version settings')} /></Setting>
  </div>
  {#if newer && !i.clientJar}
    <div class="card card-pad" style="display: grid; grid-template-columns: minmax(0,1fr) auto; gap: 16px; align-items: center">
      <span><span class="h3" style="display: block">Minecraft {newer.id} is available</span><span class="small faint" style="display: block">{compat.data ? `${compat.data.matching} of ${compat.data.total} mods already support it. We will check the rest before switching.` : 'Mods are checked before switching.'}</span></span>
      <a href={to('minecraft')} class="btn btn-accent">Check and switch</a>
    </div>
  {:else if !families.data && !i.clientJar && lastCount(newerKey)}
    <div class="card card-pad" style="display: grid; grid-template-columns: minmax(0,1fr) auto; gap: 16px; align-items: center" aria-hidden="true">
      <span><span class="h3" style="display: block"><span class="sk sk-text" style="height: 12px; width: 220px"></span></span><span class="small faint" style="display: block"><span class="sk sk-text" style="width: 360px"></span></span></span>
      <span class="sk" style="height: 32px; width: 150px; border-radius: var(--radius)"></span>
    </div>
  {/if}
</section>

<style>
  .comp{display:grid;grid-template-columns:44px minmax(0,1fr) auto;gap:14px;align-items:center;padding:14px 16px}
  .cbox{width:44px;height:44px;border-radius:9px;background:var(--raise);display:grid;place-items:center;color:var(--muted)}
</style>

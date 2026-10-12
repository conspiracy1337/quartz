<script lang="ts">
  import { api } from '$lib/api';
  import type { Instance } from '$lib/api/types';
  import { fullDate } from '$lib/format';
  import { LOADER_PRODUCTS } from '$lib/labels';
  import { query } from '$lib/query.svelte';
  import { revs } from '$lib/revs.svelte';
  import { href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';

  interface Props { instance: Instance }
  let { instance: i }: Props = $props();
  const to = (sub: string) => href({ name: 'instance', id: i.id, tab: 'version', sub });

  const families = query(() => api.versions.minecraft());
  const loaders = query(() => (i.loader ? api.versions.loaders(i.minecraft) : Promise.resolve([])));
  const runtimes = query(() => api.java.runtimes());

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

  const opt = $derived(loaders.data?.find((o) => o.kind === i.loader?.kind));
  const loaderLine = $derived.by(() => {
    if (!i.loader || !opt) return '';
    const state = opt.stable[0]?.id === i.loader.id ? 'Latest stable.' : opt.stable.some((v) => v.id === i.loader!.id) ? 'Stable build.' : opt.beta.some((v) => v.id === i.loader!.id) ? 'Beta build.' : '';
    return i.loader.kind === 'fabric' || i.loader.kind === 'quilt' ? `${state} Intermediary ${i.minecraft}.`.trim() : state;
  });

  // Runtimes are listed as "Temurin 21.0.5"; this row names the vendor in full, as the design does.
  const VENDORS: Record<string, string> = { Temurin: 'Eclipse Temurin', Zulu: 'Azul Zulu', Semeru: 'IBM Semeru' };
  // The Java this instance starts with: its own pick, else the runtime for the version's major.
  const java = $derived.by(() => {
    const list = runtimes.data ?? [];
    const own = i.settings.java.override && i.settings.java.javaPath ? list.find((r) => r.path === i.settings.java.javaPath) : undefined;
    const rt = own ?? list.find((r) => r.major === release?.java && r.source === 'Managed') ?? list.find((r) => r.major === release?.java);
    if (!rt) return { title: release ? `Java ${release.java}` : 'Java', line: 'Downloads automatically the first time you play.' };
    const parts = rt.name.split(' ');
    const version = parts.length > 1 ? parts.pop()! : rt.name;
    const vendor = parts.join(' ') || rt.name;
    return { title: `Java ${version}`, line: `${VENDORS[vendor] ?? vendor}, ${own ? 'set in this instance' : 'chosen automatically'}.` };
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

<div class="scroll">
  <div class="page page-narrow" style="gap: 20px; padding-top: 18px">
    <section class="card" aria-label="Components">
      <div class="comp"><span class="cbox" aria-hidden="true"><Icon name="brand" size={20} /></span><span><span class="h3" style="display: block">Minecraft {i.minecraft}</span><span class="small faint" style="display: block">{mcLine}</span></span><a href={to('minecraft')} class="btn btn-sm">Change</a></div>
      <div class="divider"></div>
      {#if i.loader}
        <div class="comp"><span class="cbox" style="background: #dbc18e; color: #0a0a0b" aria-hidden="true"><Icon name="fabric" size={20} sw={1.8} /></span><span><span class="h3" style="display: block">{LOADER_PRODUCTS[i.loader.kind]} {i.loader.version}</span><span class="small faint" style="display: block">{loaderLine}</span></span><span style="display: flex; gap: 6px"><button class="btn btn-sm btn-ghost" onclick={() => edit(() => api.instances.setVersion(i.id, i.minecraft, '', ''), 'Remove loader')}>Remove</button><a href={to('loader')} class="btn btn-sm">Change</a></span></div>
      {:else}
        <div class="comp"><span class="cbox" aria-hidden="true"><Icon name="fabric" size={20} sw={1.8} /></span><span><span class="h3" style="display: block">No mod loader</span><span class="small faint" style="display: block">Vanilla. Add a loader to use mods.</span></span><a href={to('loader')} class="btn btn-sm">Add</a></div>
      {/if}
      <div class="divider"></div>
      <div class="comp"><span class="cbox" aria-hidden="true"><Icon name="java" size={20} /></span><span><span class="h3" style="display: block">{java.title}</span><span class="small faint" style="display: block">{java.line}</span></span><a href={href({ name: 'settings', tab: 'java' })} class="btn btn-sm">Manage</a></div>
    </section>
    <section class="card" aria-label="Updates">
      <Setting title="Keep the loader up to date" desc="Installs new stable {i.loader ? LOADER_PRODUCTS[i.loader.kind] : 'loader'} versions when you launch."><Toggle on={i.keepLoaderUpdated} label="Keep the loader up to date" onchange={(v) => edit(() => api.instances.setVersionOptions(i.id, v, i.followLatest), 'Version settings')} /></Setting>
      <div class="divider"></div>
      <Setting title="Follow the latest Minecraft release" desc="Moves to new releases once every mod has a matching version."><Toggle on={i.followLatest} label="Follow the latest Minecraft release" onchange={(v) => edit(() => api.instances.setVersionOptions(i.id, i.keepLoaderUpdated, v), 'Version settings')} /></Setting>
    </section>
    {#if newer}
      <section class="card card-pad" aria-label="Available" style="display: grid; grid-template-columns: minmax(0,1fr) auto; gap: 16px; align-items: center">
        <span><span class="h3" style="display: block">Minecraft {newer.id} is available</span><span class="small faint" style="display: block">{compat.data ? `${compat.data.matching} of ${compat.data.total} mods already support it. We will check the rest before switching.` : 'Mods are checked before switching.'}</span></span>
        <a href={to('minecraft')} class="btn btn-accent">Check and switch</a>
      </section>
    {/if}
  </div>
</div>

<style>
  .comp{display:grid;grid-template-columns:44px minmax(0,1fr) auto;gap:14px;align-items:center;padding:14px 16px}
  .cbox{width:44px;height:44px;border-radius:9px;background:var(--raise);display:grid;place-items:center;color:var(--muted)}
</style>

<script lang="ts">
  import type { Instance } from '$lib/api/types';
  import { LOADER_PRODUCTS } from '$lib/labels';
  import { href } from '$lib/router.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';

  interface Props { instance: Instance }
  let { instance: i }: Props = $props();
  let keepLoader = $state(true);
  let followLatest = $state(false);
  const to = (sub: string) => href({ name: 'instance', id: i.id, tab: 'version', sub });
</script>

<div class="scroll">
  <div class="page page-narrow" style="gap: 20px; padding-top: 18px">
    <section class="card" aria-label="Components">
      <div class="comp"><span class="cbox" aria-hidden="true"><Icon name="brand" size={20} /></span><span><span class="h3" style="display: block">Minecraft {i.minecraft}</span><span class="small faint" style="display: block">Release, December 3, 2024. Java 21 required.</span></span><a href={to('minecraft')} class="btn btn-sm">Change</a></div>
      <div class="divider"></div>
      {#if i.loader}
        <div class="comp"><span class="cbox" style="background: #dbc18e; color: #0a0a0b" aria-hidden="true"><Icon name="fabric" size={20} sw={1.8} /></span><span><span class="h3" style="display: block">{LOADER_PRODUCTS[i.loader.kind]} {i.loader.version}</span><span class="small faint" style="display: block">Latest stable. Intermediary {i.minecraft}.</span></span><span style="display: flex; gap: 6px"><button class="btn btn-sm btn-ghost">Remove</button><a href={to('loader')} class="btn btn-sm">Change</a></span></div>
      {:else}
        <div class="comp"><span class="cbox" aria-hidden="true"><Icon name="fabric" size={20} sw={1.8} /></span><span><span class="h3" style="display: block">No mod loader</span><span class="small faint" style="display: block">Vanilla. Add a loader to use mods.</span></span><a href={to('loader')} class="btn btn-sm">Add</a></div>
      {/if}
      <div class="divider"></div>
      <div class="comp"><span class="cbox" aria-hidden="true"><Icon name="java" size={20} /></span><span><span class="h3" style="display: block">Java 21.0.5</span><span class="small faint" style="display: block">Eclipse Temurin, chosen automatically.</span></span><a href={href({ name: 'settings', tab: 'java' })} class="btn btn-sm">Manage</a></div>
    </section>
    <section class="card" aria-label="Updates">
      <Setting title="Keep the loader up to date" desc="Installs new stable {i.loader ? LOADER_PRODUCTS[i.loader.kind] : 'loader'} versions when you launch."><Toggle bind:on={keepLoader} label="Keep the loader up to date" /></Setting>
      <div class="divider"></div>
      <Setting title="Follow the latest Minecraft release" desc="Moves to new releases once every mod has a matching version."><Toggle bind:on={followLatest} label="Follow the latest Minecraft release" /></Setting>
    </section>
    <section class="card card-pad" aria-label="Available" style="display: grid; grid-template-columns: minmax(0,1fr) auto; gap: 16px; align-items: center">
      <span><span class="h3" style="display: block">Minecraft 1.21.5 is available</span><span class="small faint" style="display: block">41 of 48 mods already support it. We will check the rest before switching.</span></span>
      <a href={to('minecraft')} class="btn btn-accent">Check and switch</a>
    </section>
  </div>
</div>

<style>
  .comp{display:grid;grid-template-columns:44px minmax(0,1fr) auto;gap:14px;align-items:center;padding:14px 16px}
  .cbox{width:44px;height:44px;border-radius:9px;background:var(--raise);display:grid;place-items:center;color:var(--muted)}
</style>

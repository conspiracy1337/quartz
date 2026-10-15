<script lang="ts">
  import { api } from '$lib/api';
  import { plural } from '$lib/format';
  import { query, skeletonRows } from '$lib/query.svelte';
  import { groupsFor } from '$lib/settings/registry';
  import SettingsGroup from '$lib/settings/SettingsGroup.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Menu, { below } from '$lib/ui/Menu.svelte';
  import { prefs } from '$lib/prefs.svelte';
  import SettingsTab from './SettingsTab.svelte';

  import type { JavaRuntime } from '$lib/api/types';
  import { session } from '$lib/session.svelte';
  import { tasks } from '$lib/tasks.svelte';

  const runtimes = query(() => api.java.runtimes(), 'java.runtimes');
  const defs = groupsFor('java').flatMap((g) => g.defs);
  let changed = $state<JavaRuntime[] | null>(null);
  let busy = $state(false);
  const list = $derived(changed ?? runtimes.data);
  const majors = query(() => api.java.majors());
  /** Automatic picking off: one runtime is every instance's Java (its menu offers it). */
  const manual = $derived(prefs.value?.java.autoPick === false);
  let menu = $state<{ x: number; y: number; j: JavaRuntime } | null>(null);
  let pick = $state<{ x: number; y: number } | null>(null);
  // A menu row reads its runtime before the menu closes (closing drops `menu`).
  function fromMenu(fn: (j: JavaRuntime) => Promise<unknown>) {
    const j = menu?.j;
    menu = null;
    if (j) void fn(j);
  }

  async function act(fn: () => Promise<JavaRuntime[]>) {
    busy = true;
    try {
      changed = await fn();
    } catch (e) {
      session.fail('Java', e);
    } finally {
      busy = false;
    }
  }
  async function download(major: number) {
    pick = null;
    busy = true;
    try {
      await api.java.download(major);
      await tasks.load();
    } catch (e) {
      session.fail('Java download', e);
    } finally {
      busy = false;
    }
  }
</script>

<SettingsTab tab="java" gap={20}>
  <section style="display: flex; flex-direction: column; gap: 8px" aria-label="Runtimes">
    <div style="display: flex; justify-content: space-between; align-items: center"><h2 class="h3">Java runtimes</h2><div style="display: flex; gap: 6px"><button class="btn btn-sm" disabled={busy} onclick={() => act(api.java.scan)}>Scan again</button><button class="btn btn-sm" disabled={busy} onclick={() => act(api.java.addFromFolder)}>Add from folder</button><button class="btn btn-sm btn-accent" disabled={busy} aria-haspopup="menu" onclick={(e) => (pick = below(e))}>Download</button></div></div>
    <div class="list">
      {#each list ?? [] as j (j.path)}
        <div class="row jr">
          <span class="maj">{j.major}</span>
          <span style="min-width: 0"><span style="display: flex; gap: 8px; align-items: center"><span class="trunc" style="font-weight: 500">{j.name}</span><span class="badge" class:badge-accent={j.source === 'Managed'}>{j.source}</span>{#if j.default && manual}<span class="badge badge-accent">Default</span>{/if}</span><span class="trunc small faint mono" style="display: block">{j.path}</span></span>
          <span class="small muted opt">{j.usedBy ? plural(j.usedBy, 'instance') : 'Not used'}</span>
          <button class="btn btn-icon btn-sm btn-ghost" aria-label="More for {j.name}" aria-haspopup="menu" onclick={(e) => (menu = { ...below(e), j })}><Icon name="more" size={14} /></button>
        </div>
      {:else}
        {#if list}
          <div class="row empty jr-empty"><span>No Java found. Download one now, or Quartz downloads the right version when you first play.</span></div>
        {:else}
        {#each { length: skeletonRows('java.runtimes', 6) } as _, k (k)}<div class="row jr" aria-hidden="true"><div class="sk" style="width: 44px; height: 32px; border-radius: 6px"></div><div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: 35%"></div><div class="sk" style="height: 10px; width: 50%"></div></div><div class="sk opt" style="height: 10px; width: 70px"></div><div></div></div>{/each}
        {/if}
      {/each}
    </div>
  </section>
  <SettingsGroup {defs} />
</SettingsTab>

{#if menu}
  {@const j = menu.j}
  <Menu x={menu.x} y={menu.y} label="More for {j.name}" onclose={() => (menu = null)}>
    <button class="mi" role="menuitem" onclick={() => fromMenu((r) => api.java.showInFolder(r.id).catch((e) => session.fail('Show in folder', e)))}><Icon name="folder" size={14} /><span>Show in folder</span><span></span></button>
    {#if manual && !j.default}<button class="mi" role="menuitem" onclick={() => fromMenu((r) => act(() => api.java.setDefault(r.id)))}><Icon name="check" size={14} /><span>Use for every instance</span><span></span></button>{/if}
    {#if j.removable}<button class="mi" role="menuitem" onclick={() => fromMenu((r) => act(() => api.java.remove(r.id)))}><Icon name="trash" size={14} /><span>Remove</span><span></span></button>
    {:else if j.forgettable}<button class="mi" role="menuitem" onclick={() => fromMenu((r) => act(() => api.java.forget(r.id)))}><Icon name="trash" size={14} /><span>Forget</span><span></span></button>{/if}
  </Menu>
{/if}
{#if pick}
  <Menu x={pick.x} y={pick.y} label="Download Java" onclose={() => (pick = null)}>
    {#each majors.data ?? [] as major (major)}<button class="mi" role="menuitem" onclick={() => download(major)}><Icon name="download" size={14} /><span>Java {major}</span><span></span></button>{/each}
  </Menu>
{/if}

<style>
  .jr{grid-template-columns:44px minmax(0,1fr) 110px 28px;height:60px}
  @media (max-width:1100px){.jr{grid-template-columns:44px minmax(0,1fr) 28px}.jr .opt{display:none}}
  .jr-empty{height:60px;padding:0 var(--inset);flex-direction:row}
  .maj{width:44px;height:32px;border-radius:6px;background:var(--raise);display:grid;place-items:center;font-family:var(--mono);font-weight:600}
</style>

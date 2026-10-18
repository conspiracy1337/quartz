<script lang="ts">
  import { api } from '../api';
  import type { ContentKind, InstallDep, InstallPlan, Src } from '../api/types';
  import { bytes, errorText, gb, plural } from '../format';
  import { LOADER_NAMES, PROJECT_TYPES } from '../labels';
  import { downloading } from '../instanceActions';
  import { artOf, memoryShort } from '../look';
  import { prefs } from '../prefs.svelte';
  import { query } from '../query.svelte';
  import { session } from '../session.svelte';
  import { tasks } from '../tasks.svelte';
  import Check from '../ui/Check.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Icon from '../ui/Icon.svelte';
  import Select from '../ui/Select.svelte';
  import Tile from '../ui/Tile.svelte';

  /** Install a project (a given version, or the newest that fits) into an instance, with its dependencies; `plan` is one already made. */
  interface Props { ref: string; source: Src; instanceId: string; content: ContentKind; version?: string | undefined; plan?: InstallPlan | undefined }
  let { ref, source, instanceId, content, version = '', plan: made }: Props = $props();

  const inst = query(() => api.instances.get(instanceId));
  const info = query(() => api.app.info(), 'app.info');
  const tabInfo = query(() => (content === 'datapacks' ? api.content.info(instanceId) : Promise.resolve(undefined)));
  let world = $state('');
  const worlds = $derived(tabInfo.data?.worlds ?? []);
  $effect(() => {
    if (!world && worlds[0]) world = worlds[0];
  });
  const waiting = $derived(content === 'datapacks' && !world);
  const plan = query(() => (waiting ? Promise.resolve(undefined) : made ? Promise.resolve(made) : api.browse.planInstall(instanceId, content, world, ref, source, version)));
  let optional = $state<Record<string, boolean>>({});
  let busy = $state(false);

  const p = $derived(plan.data);
  const noun = $derived(PROJECT_TYPES.find((t) => t.content === content)?.noun ?? 'mod');
  const count = $derived(p ? 1 + p.deps.filter((d) => (d.required && !d.installed && !d.unavailable) || (!d.required && optional[d.id] !== false)).length : 1);
  function depNote(d: InstallDep): string {
    if (!d.required) return d.summary ? `Optional, ${d.summary}` : 'Optional';
    if (d.unavailable) return 'Required, no build for this version';
    if (d.off) return 'Required, turned off; will be turned on';
    return d.installed ? 'Required, already installed' : 'Required';
  }
  // Only a mod is built for a loader; a pack fits by the game version alone.
  const loader = $derived(content === 'mods' && inst.data?.loader ? `, ${LOADER_NAMES[inst.data.loader.kind]}` : '');
  // Composed: shown only when the mods after this install would want more memory than the instance has.
  const suggest = $derived(p && content === 'mods' && prefs.value?.java.suggestMemory !== false ? memoryShort(p.mods + count, info.data?.memoryMb ?? 0, inst.data?.memoryMB.max ?? 0) : 0);
  const setGb = $derived(gb(inst.data?.memoryMB.max ?? 0));

  async function install() {
    if (!p) return;
    busy = true;
    try {
      const picked = p.deps.filter((d) => !d.required && optional[d.id] !== false).map((d) => d.id);
      const job = await api.browse.applyInstall(p.planId, picked);
      session.close();
      await downloading(job);
    } catch (e) {
      busy = false;
      session.notify({ tone: 'error', subject: `Couldn't install ${p.name}`, text: errorText(e) });
    }
  }
</script>

<Dialog title="Install {p?.name ?? ''}" onclose={() => session.close()}>
  <div class="kv" style="grid-template-columns: 84px minmax(0,1fr)">
    <span>Instance</span><span class="trunc">{inst.data?.name ?? ''}</span>
    {#if content === 'datapacks'}
      <!-- Composed: data packs go into one world. -->
      <span>World</span>{#if worlds.length}<Select bind:value={world} options={worlds.map((w) => ({ value: w, label: w }))} label="World" style="width: 100%" />{:else}<span class="muted">{tabInfo.data ? 'Create a world first' : ''}</span>{/if}
    {/if}
    <span>Version</span>
    {#if p && !p.version}<span class="muted">No version for {inst.data?.minecraft ?? ''}{loader}</span>
    {:else}<span class="mono trunc">{p?.version ?? ''} for {inst.data?.minecraft ?? ''}{loader}</span>{/if}
    {#if p?.replaces}<span>Installed</span><span class="mono trunc">{p.replaces}</span>{/if}
  </div>
  {#if p?.deps.length}
    <div style="display: flex; flex-direction: column; gap: 6px">
      <span class="field-label">Also installs</span>
      <div class="list">
        {#each p.deps as d (d.id)}
          <div class="dep">
            {#if d.required}<Check on={true} />{:else}<Check on={optional[d.id] !== false} label="Include {d.name}" onchange={(v) => (optional[d.id] = v)} />{/if}
            <Tile art={d.art ?? artOf(d.name, d.icon)} style="width: 28px; height: 28px; flex-basis: 28px" />
            <span style="min-width: 0"><span class="trunc" style="display: block">{d.name}</span><span class="small faint trunc" style="display: block">{depNote(d)}</span></span>
            {#if d.unavailable}<span class="badge badge-warn">Missing</span>{:else if d.installed}<span class="badge badge-accent">Installed</span>{:else}<span class="mono small muted">{bytes(d.size)}</span>{/if}
          </div>
        {/each}
      </div>
    </div>
  {:else if plan.loading && !p}
    <div class="list" aria-hidden="true"><div class="dep"><div class="sk" style="width: 16px; height: 16px"></div><div class="sk" style="width: 28px; height: 28px"></div><div class="sk" style="height: 12px; width: 40%"></div><span></span></div></div>
  {/if}
  {#if suggest}
    <div class="card" style="padding: 10px 12px; display: grid; grid-template-columns: 16px minmax(0,1fr); gap: 10px; border-color: color-mix(in srgb, var(--info) 30%, var(--line))">
      <Icon name="info" size={16} sw={1.8} color="var(--info)" />
      <span class="small muted">{suggest} GB suits {plural(p!.mods + count, 'mod')}. This instance is set to {setGb} GB.</span>
    </div>
  {/if}
  {#if plan.error}<p class="small" style="margin: 0; color: var(--danger)">{errorText(plan.error)}</p>{/if}
  {#snippet foot()}<button class="btn" onclick={() => session.close()}>Cancel</button><button class="btn btn-accent" disabled={!p?.version || busy} onclick={install}>Install {plural(count, noun)}</button>{/snippet}
</Dialog>

<style>
  .dep{display:grid;grid-template-columns:16px 28px minmax(0,1fr) auto;gap:10px;align-items:center;height:48px;padding:0 12px}
</style>

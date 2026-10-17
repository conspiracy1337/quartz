<script lang="ts">
  import { NEW_INSTANCE_SHARES } from '$lib/shares';
  import { api } from '$lib/api';
  import { bytes, gbField } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import VersionPicker from '$lib/components/VersionPicker.svelte';
  import { revs } from '$lib/revs.svelte';
  import { session } from '$lib/session.svelte';
  import { go, href } from '$lib/router.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Seg from '$lib/ui/Seg.svelte';
  import LoaderIcon from '$lib/ui/LoaderIcon.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';
  import ToggleRow from '$lib/ui/ToggleRow.svelte';
  import LookEditor from '$lib/components/LookEditor.svelte';
  import type { ClientJarPick, Look } from '$lib/api/types';

  // The loader segment, in the design's order. Fabric and Quilt name their newest stable
  // build; NeoForge and Forge keep the design's wording.
  const LOADERS = [
    { value: 0, kind: 'none', label: 'None', short: 'no mod loader' },
    { value: 1, kind: 'fabric', label: 'Fabric', short: 'Fabric' },
    { value: 2, kind: 'quilt', label: 'Quilt', short: 'Quilt' },
    { value: 3, kind: 'neoforge', label: 'NeoForge', short: 'NeoForge' },
    { value: 4, kind: 'forge', label: 'Forge', short: 'Forge' },
  ] as const;

  // A custom client jar runs without a mod loader, on the version it was built from.
  let jar = $state<ClientJarPick | null>(null);
  let name = $state('');
  let version = $state('');
  let loader = $state(1); // Fabric, as the design starts
  let creating = $state(false);
  let more = $state(false);
  let sandbox = $state(true);
  // Starts on only where games can be sandboxed: a sandboxed instance never
  // launches unsandboxed.
  void api.sandbox.status().then((st) => { if (!st.available) sandbox = false; });
  let shares = $state<Record<number, boolean>>({ 0: true, 2: true, 4: true, 5: true });
  let look = $state<Look>({ art: { background: '#6b8afd', initial: '' }, icon: '', banner: '' });
  // Empty follows the global memory, shown as the placeholder.
  let memory = $state('');
  const l = $derived(LOADERS[loader]!);
  const loaderOptions = $derived(LOADERS.map((o) => ({ ...o, disabled: !!jar && o.kind !== 'none' })));
  const options = query(() => (version ? api.versions.loaders(version) : Promise.resolve([])));
  const opt = $derived(options.data?.find((o) => o.kind === l.kind));
  const build = $derived(opt?.stable[0] ?? opt?.beta[0]);
  const noBuild = $derived(l.kind !== 'none' && !!options.data && !build);
  const line = $derived(
    l.kind === 'none' ? 'Vanilla, no mod loader.'
    : noBuild ? `${l.label} has no build for ${version} yet.`
    : l.kind === 'neoforge' ? 'Latest stable NeoForge for this version.'
    : l.kind === 'forge' ? 'Latest recommended Forge for this version.'
    : build ? `${l.label} ${build.name}, latest stable.` : '');
  const loaderVersion = $derived(l.kind === 'none' ? '' : (build?.id ?? ''));
  const estimate = query(() => (version && !noBuild ? api.versions.estimate(version, l.kind, loaderVersion) : Promise.resolve(0)));
  const download = $derived(estimate.data === undefined ? '' : estimate.data > 0 ? `Downloads about ${bytes(estimate.data)} the first time you play.` : 'Everything it needs is already downloaded.');
  const meta = $derived(version ? `${version}${l.kind === 'none' || jar ? '' : `, ${l.label}`}` : '');

  async function pickJar() {
    try {
      const p = await api.instances.chooseClientJar();
      if (!p.sha1) return;
      jar = p;
      loader = 0;
    } catch (e) {
      session.fail('Custom jar', e);
    }
  }

  async function create() {
    creating = true;
    try {
      const n = name.trim();
      const made = await api.instances.create({
        name: n, art: look.art, icon: look.icon, banner: look.banner, minecraft: version,
        loader: l.kind === 'none' ? '' : l.kind, loaderVersion, sandbox, memoryMB: memory.trim() ? gbField(memory) : 0, shares: NEW_INSTANCE_SHARES.filter((_, i) => shares[i]).map((s) => s.id),
        clientJar: jar?.sha1 ?? '',
      });
      revs.bump('instances');
      revs.bump('sync');
      go({ name: 'instance', id: made.id, tab: 'overview' });
    } catch (e) {
      session.fail('New instance', e);
    } finally {
      creating = false;
    }
  }
</script>

<div class="scroll">
  <div style="max-width: 760px; margin: 0 auto; padding: 28px 24px 32px; display: flex; flex-direction: column; gap: 18px">
    <div style="display: flex; align-items: center; gap: 8px"><h1 class="h1" style="margin-right: auto">New instance</h1><a href={href({ name: 'import' })} class="btn"><Icon name="upload" size={14} sw={1.8} />Import file</a><a href={href({ name: 'browse', type: 'modpack' })} class="btn">Modpack</a></div>
    <div class="field"><span class="field-label">Mod loader</span>
      {#snippet mark(v: number)}<LoaderIcon kind={LOADERS[v]?.kind ?? 'none'} size={14} />{/snippet}
      <Seg options={loaderOptions} bind:value={loader} label="Mod loader" icon={mark} fill center />
      <span class="small faint">{jar ? 'Custom jars run without a mod loader.' : `${line} You can change it later under Version and loader.`}</span>
    </div>
    <div class="field"><span class="field-label">Minecraft version</span><VersionPicker start={jar?.minecraft ?? ''} startBadge="From jar" onpick={(v) => (version = v)} /></div>
    <div class="field"><span class="field-label">Appearance</span>
      <LookEditor bind:look {name} {meta} loader={jar ? '' : l.kind}>
        <label class="field"><span class="field-label">Instance name</span><span class="input"><input bind:value={name} placeholder="Enter Instance Name" maxlength="64" /></span></label>
      </LookEditor>
    </div>
    <section class="field" aria-label="Share with other instances">
      <span class="field-label">Share with other instances</span>
      <div class="card tgl-grid">
        {#each NEW_INSTANCE_SHARES as s, i (s.id)}<ToggleRow name={s.label} on={!!shares[i]} onchange={(v) => (shares[i] = v)} />{/each}
      </div>
      <span class="field-help">Shared keybinds only overwrite the keys you picked in Syncing, not the whole options file.</span>
    </section>
    <button class="adv" class:open={more} onclick={() => (more = !more)} aria-expanded={more}><Icon name="chevron-right" size={12} sw={2.2} />More options</button>
    {#if more}
      <div class="card">
        <Setting title="Memory" desc="Automatic uses the global setting."><label class="input" style="width: 128px"><span class="sr">Memory</span><input class="tnum" bind:value={memory} placeholder="Automatic" inputmode="numeric" /><span class="suffix">GB</span></label></Setting>
        <div class="divider"></div>
        <Setting title="Sandbox" desc="Recommended for mods from unknown sources."><Toggle bind:on={sandbox} label="Sandbox" /></Setting>
        <div class="divider"></div>
        <Setting title="Game jar" oneLine desc={jar ? `${jar.name} · ${bytes(jar.size)}${jar.minecraft ? `, built for ${jar.minecraft}` : ''}` : 'Use a modified version jar. Mods will be unavailable.'}>
          {#if jar}<button class="btn btn-sm btn-ghost" onclick={() => (jar = null)}>Remove</button>{:else}<button class="btn btn-sm" onclick={pickJar}>Choose jar</button>{/if}
        </Setting>
      </div>
    {/if}
    <button class="btn btn-accent btn-lg btn-block" disabled={creating || !name.trim() || !version || noBuild} onclick={create}>Create</button>
    <p class="small faint" style="margin: 0; text-align: center">Minecraft {version}{jar ? ` with ${jar.name}, no mod loader` : `, ${l.short}`}. {download}</p>
  </div>
</div>

<style>
  .adv{display:flex;align-items:center;gap:8px;border:0;background:transparent;color:var(--muted);padding:0;height:28px}
  .adv :global(svg){transition:transform 160ms var(--ease)}
  .adv.open :global(svg){transform:rotate(90deg)}
</style>

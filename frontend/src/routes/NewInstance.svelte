<script lang="ts">
  import { api } from '$lib/api';
  import { bytes } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import VersionPicker from '$lib/components/VersionPicker.svelte';
  import { revs } from '$lib/revs.svelte';
  import { session } from '$lib/session.svelte';
  import { go, href } from '$lib/router.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Seg from '$lib/ui/Seg.svelte';
  import Select from '$lib/ui/Select.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';
  import ToggleRow from '$lib/ui/ToggleRow.svelte';
  import InstanceMark from '$lib/ui/InstanceMark.svelte';
  import PromptDialog from '$lib/ui/PromptDialog.svelte';
  import IconDialog from '$lib/overlays/IconDialog.svelte';
  import { prefs } from '$lib/prefs.svelte';
  import type { Art } from '$lib/api/types';

  // The loader segment, in the design's order. Fabric and Quilt name their newest stable
  // build; NeoForge and Forge keep the design's wording.
  const LOADERS = [
    { value: 0, kind: 'none', label: 'None', short: 'no mod loader' },
    { value: 1, kind: 'fabric', label: 'Fabric', short: 'Fabric' },
    { value: 2, kind: 'quilt', label: 'Quilt', short: 'Quilt' },
    { value: 3, kind: 'neoforge', label: 'NeoForge', short: 'NeoForge' },
    { value: 4, kind: 'forge', label: 'Forge', short: 'Forge' },
  ] as const;
  const SHARES = ['Keybinds', 'Game options', 'Servers', 'Worlds', 'Resource packs', 'Shaders', 'Screenshots', 'Hotbars'];

  // The name follows the picked version until the user types their own.
  let typed = $state<string | null>(null);
  let version = $state('');
  const name = $derived(typed ?? version);
  let loader = $state(1); // Fabric, as the design starts
  let creating = $state(false);
  let more = $state(false);
  let group = $state('none');
  let naming = $state(false);
  let sandbox = $state(true);
  let shares = $state<Record<number, boolean>>({ 0: true, 2: true, 4: true, 5: true });
  let art = $state<Art>({ background: '#6b8afd', initial: '' });
  let icon = $state('');
  let picking = $state(false);
  const l = $derived(LOADERS[loader]!);
  const options = query(() => (version ? api.versions.loaders(version) : Promise.resolve([])));
  const groups = query(() => api.instances.groups());
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
  const initial = $derived(art.initial || name.trim().charAt(0).toUpperCase());
  const groupOptions = $derived([{ value: 'none', label: 'None' }, ...(groups.data ?? []).map((g) => ({ value: g.name, label: g.name })), { value: '\u0000new', label: 'New group' }]);
  const memory = $derived(prefs.value ? `Automatic, ${Math.round(prefs.value.java.memoryMaxMb / 1024)} GB` : 'Automatic');

  function pickGroup(v: string) {
    if (v === '\u0000new') {
      group = 'none';
      naming = true;
    }
  }

  async function create() {
    creating = true;
    try {
      const n = name.trim();
      const made = await api.instances.create({
        name: n, art: { background: art.background, initial }, icon, minecraft: version,
        loader: l.kind === 'none' ? '' : l.kind, loaderVersion, group: group === 'none' ? '' : group, sandbox, shares: SHARES.filter((_, i) => shares[i]),
      });
      revs.bump('instances');
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
    <div style="display: grid; grid-template-columns: 56px minmax(0,1fr); gap: 14px; align-items: center">
      <button class="ico" style="width: 56px; height: 56px; border-radius: 12px; background: {art.background}; border: 0; font-size: 22px" aria-label="Change icon" onclick={() => (picking = true)}><InstanceMark art={{ background: art.background, initial }} {icon} /></button>
      <label class="field"><span class="sr">Name</span><input class="name" value={name} oninput={(e) => (typed = e.currentTarget.value)} maxlength="64" /></label>
    </div>
    <div class="field"><span class="field-label">Minecraft version</span><VersionPicker onpick={(v) => (version = v)} /></div>
    <div class="field"><span class="field-label">Mod loader</span>
      <Seg options={LOADERS} bind:value={loader} label="Mod loader" fill center />
      <span class="small faint">{line} You can change it later under Version and loader.</span>
    </div>
    <section class="field" aria-label="Share with other instances">
      <span class="field-label">Share with other instances</span>
      <div class="card tgl-grid">
        {#each SHARES as s, i (s)}<ToggleRow name={s} on={!!shares[i]} onchange={(v) => (shares[i] = v)} />{/each}
      </div>
      <span class="field-help">Shared keybinds only overwrite the keys you picked in Syncing, not the whole options file.</span>
    </section>
    <button class="adv" class:open={more} onclick={() => (more = !more)} aria-expanded={more}><Icon name="chevron-right" size={12} sw={2.2} />More options</button>
    {#if more}
      <div class="card">
        <Setting title="Group"><Select bind:value={group} options={groupOptions} label="Group" style="width: 160px" onchange={pickGroup} /></Setting>
        <div class="divider"></div>
        <Setting title="Memory"><span class="muted">{memory}</span></Setting>
        <div class="divider"></div>
        <Setting title="Sandbox" desc="Recommended for mods from unknown sources."><Toggle bind:on={sandbox} label="Sandbox" /></Setting>
      </div>
    {/if}
    <button class="btn btn-accent btn-lg btn-block" disabled={creating || !name.trim() || !version || noBuild} onclick={create}>Create</button>
    <p class="small faint" style="margin: 0; text-align: center">Minecraft {version}, {l.short}. {download}</p>
  </div>
</div>

{#if picking}
  <IconDialog art={{ background: art.background, initial }} {icon} choose={api.instances.chooseIcon} save={async (a, pic) => { art = { background: a.background, initial: a.initial === name.trim().charAt(0).toUpperCase() ? '' : (a.initial ?? '') }; icon = pic; }} onclose={() => (picking = false)} />
{/if}
{#if naming}
  <PromptDialog title="New group" label="Name" help="Instances can be moved between groups later in their settings." action="Create group" maxlength={32}
    submit={async (v) => { const g = await api.instances.createGroup(v); revs.bump('instances'); group = g.name; }} onclose={() => (naming = false)} />
{/if}

<style>
  .name{all:unset;font-size:20px;line-height:28px;font-weight:600;letter-spacing:-0.02em;border-bottom:1px solid var(--line-2);padding-bottom:6px}
  .adv{display:flex;align-items:center;gap:8px;border:0;background:transparent;color:var(--muted);padding:0;height:28px}
  .adv :global(svg){transition:transform 160ms var(--ease)}
  .adv.open :global(svg){transform:rotate(90deg)}
</style>

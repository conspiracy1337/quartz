<script lang="ts">
  import VersionPicker from '$lib/components/VersionPicker.svelte';
  import { go, href } from '$lib/router.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Seg from '$lib/ui/Seg.svelte';
  import Select from '$lib/ui/Select.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';
  import ToggleRow from '$lib/ui/ToggleRow.svelte';

  const LOADERS = [
    { value: 0, label: 'None', line: 'Vanilla, no mod loader.', short: 'no mod loader' },
    { value: 1, label: 'Fabric', line: 'Fabric 0.16.10, latest stable.', short: 'Fabric' },
    { value: 2, label: 'Quilt', line: 'Quilt 0.27.1, latest stable.', short: 'Quilt' },
    { value: 3, label: 'NeoForge', line: 'Latest stable NeoForge for this version.', short: 'NeoForge' },
    { value: 4, label: 'Forge', line: 'Latest recommended Forge for this version.', short: 'Forge' },
  ];
  const SHARES = ['Keybinds', 'Game options', 'Servers', 'Worlds', 'Resource packs', 'Shaders', 'Screenshots', 'Hotbars'];

  let name = $state('Survival 1.21.5');
  let version = $state('');
  let loader = $state(1);
  let more = $state(false);
  let group = $state('none');
  let sandbox = $state(true);
  let shares = $state<Record<number, boolean>>({ 0: true, 2: true, 4: true, 5: true });
  const l = $derived(LOADERS[loader]!);
</script>

<div class="scroll">
  <div style="max-width: 760px; margin: 0 auto; padding: 28px 24px 32px; display: flex; flex-direction: column; gap: 18px">
    <div style="display: flex; align-items: center; gap: 8px"><h1 class="h1" style="margin-right: auto">New instance</h1><a href={href({ name: 'import' })} class="btn"><Icon name="upload" size={14} sw={1.8} />Import file</a><a href={href({ name: 'browse' })} class="btn">Modpack</a></div>
    <div style="display: grid; grid-template-columns: 56px minmax(0,1fr); gap: 14px; align-items: center">
      <button class="ico" style="width: 56px; height: 56px; border-radius: 12px; background: #6b8afd; border: 0; font-size: 22px" aria-label="Change icon">{name.trim().charAt(0).toUpperCase()}</button>
      <label class="field"><span class="sr">Name</span><input class="name" bind:value={name} maxlength="64" /></label>
    </div>
    <div class="field"><span class="field-label">Minecraft version</span><VersionPicker onpick={(v) => (version = v)} /></div>
    <div class="field"><span class="field-label">Mod loader</span>
      <Seg options={LOADERS} bind:value={loader} label="Mod loader" fill />
      <span class="small faint">{l.line} You can change it later under Version and loader.</span>
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
        <Setting title="Group"><Select bind:value={group} options={[{ value: 'none', label: 'None' }, { value: 'modded', label: 'Modded' }]} label="Group" style="width: 160px" /></Setting>
        <div class="divider"></div>
        <Setting title="Memory"><span class="muted">Automatic, 4 GB</span></Setting>
        <div class="divider"></div>
        <Setting title="Sandbox" desc="Recommended for mods from unknown sources."><Toggle bind:on={sandbox} label="Sandbox" /></Setting>
      </div>
    {/if}
    <button class="btn btn-accent btn-lg btn-block" disabled={!name.trim()} onclick={() => go({ name: 'instance', id: 'fo', tab: 'overview' })}>Create</button>
    <p class="small faint" style="margin: 0; text-align: center">Minecraft {version}, {l.short}. Downloads about 180 MB the first time you play.</p>
  </div>
</div>

<style>
  .name{all:unset;font-size:20px;line-height:28px;font-weight:600;letter-spacing:-0.02em;border-bottom:1px solid var(--line-2);padding-bottom:6px}
  .adv{display:flex;align-items:center;gap:8px;border:0;background:transparent;color:var(--muted);padding:0;height:28px}
  .adv :global(svg){transition:transform 160ms var(--ease)}
  .adv.open :global(svg){transform:rotate(90deg)}
</style>

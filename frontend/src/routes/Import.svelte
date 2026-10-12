<script lang="ts">
  import { go, href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';

  let link = $state('https://modrinth.com/modpack/cobblemon-fabric');
  let name = $state('Cobblemon Official Modpack');

  function install() {
    session.tasksRunning = true;
    go({ name: 'downloads' });
  }
</script>

<div class="topbar">
  <a href={href({ name: 'library' })} class="btn btn-sm btn-ghost btn-icon" aria-label="Back"><Icon name="chevron-left" size={14} sw={2} /></a>
  <h1 class="h2" style="margin-right: auto">Import an instance</h1>
</div>
<div class="scroll">
  <div class="page nc">
    <section aria-label="Source" style="display: flex; flex-direction: column; gap: 14px">
      <div class="drop">
        <Icon name="upload" size={22} />
        <span>Drop a .mrpack, CurseForge zip or instance zip</span>
        <button class="btn btn-sm">Choose file</button>
      </div>
      <label class="field"><span class="field-label">Or paste a link</span><span class="input" style="height: 36px"><Icon name="link" size={14} sw={1.8} /><input bind:value={link} /></span><span class="field-help">Modrinth, CurseForge, FTB and Technic links work. So do direct file links.</span></label>
      <div class="small faint">Formats: Modrinth .mrpack, CurseForge zip, Prism and MultiMC zip, Technic, FTB.</div>
    </section>
    <section class="card" aria-label="Detected">
      <div style="display: grid; grid-template-columns: 56px minmax(0,1fr); gap: 12px; align-items: center; padding: 16px">
        <span class="ico" style="width: 56px; height: 56px; border-radius: 12px; background: #f2a25c; font-size: 20px">C</span>
        <div style="min-width: 0"><h2 class="h2 trunc">Cobblemon Official Modpack</h2><div class="small muted">v1.6.1 for Fabric 1.21.1</div></div>
      </div>
      <div class="divider"></div>
      <div class="card-pad" style="display: flex; flex-direction: column; gap: 12px">
        <div class="kv small" style="grid-template-columns: 96px minmax(0,1fr); row-gap: 8px"><span>Source</span><span>Modrinth</span><span>Mods</span><span class="tnum">74, 71 MB</span><span>Overrides</span><span class="tnum">config, 212 files</span></div>
        <div class="card" style="padding: 10px 12px; border-color: color-mix(in srgb, var(--warn) 35%, var(--line)); display: grid; grid-template-columns: 16px minmax(0,1fr); gap: 10px">
          <Icon name="warn" size={16} sw={1.8} color="var(--warn)" />
          <span class="small"><span style="display: block; font-weight: 500">1 jar in overrides is not from Modrinth</span><span class="faint" style="display: block">mods/cobblemon-extras-hotfix.jar. It will run inside the sandbox.</span></span>
        </div>
        <label class="field"><span class="field-label">Instance name</span><span class="input"><input bind:value={name} /></span></label>
        <button class="btn btn-accent btn-block" disabled={!name.trim()} onclick={install}>Install</button>
      </div>
    </section>
  </div>
</div>

<style>
  .drop{height:150px;border:1px dashed var(--line-2);border-radius:12px;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:8px;color:var(--muted);background:var(--panel)}
  .nc{display:grid;grid-template-columns:minmax(0,1fr) 340px;gap:20px;align-items:start}
  @media (max-width:1100px){.nc{grid-template-columns:minmax(0,1fr)}}
</style>

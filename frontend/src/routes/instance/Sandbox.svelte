<script lang="ts">
  import type { Instance } from '$lib/api/types';
  import { href } from '$lib/router.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import type { IconName } from '$lib/ui/icons';
  import Seg from '$lib/ui/Seg.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';

  interface Props { instance: Instance }
  let { instance: i }: Props = $props();

  type Level = 'rw' | 'ro' | 'no' | 'on';
  const LEVELS: Record<Level, [string, string]> = { rw: ['Read and write', 'l-rw'], ro: ['Read only', 'l-ro'], no: ['Blocked', 'l-no'], on: ['Allowed', 'l-rw'] };
  const rules: { name: string; path: string; level: Level; icon: IconName }[] = $derived([
    { name: 'This instance', path: `${i.folder}/`, level: 'rw', icon: 'folder' },
    { name: 'Shared libraries and assets', path: 'libraries/, assets/', level: 'ro', icon: 'folder' },
    { name: 'Java runtime', path: 'java/temurin-21.0.5/', level: 'ro', icon: 'folder' },
    { name: 'Synced folders', path: 'sync/options, sync/servers', level: 'rw', icon: 'folder' },
    { name: 'Internet', path: 'Needed for servers and skins', level: 'on', icon: 'globe' },
    { name: 'Discord status', path: 'Discord IPC socket', level: 'on', icon: 'chat' },
    { name: 'Graphics and audio', path: 'GPU, PipeWire, Wayland', level: 'on', icon: 'gpu' },
    { name: 'Your home folder', path: '~/ and other instances', level: 'no', icon: 'house' },
  ]);
  const PRESETS = [{ value: 'standard', label: 'Standard' }, { value: 'strict', label: 'Strict' }, { value: 'custom', label: 'Custom' }] as const;

  let on = $state(true);
  let preset = $state<string>('standard');
  let checkJars = $state(true);
  let scan = $state(true);
</script>

<div class="scroll">
  <div class="page page-narrow" style="gap: 20px; padding-top: 18px">
    <div style="display: flex; align-items: center; gap: 8px"><a href={href({ name: 'instance', id: i.id, tab: 'settings' })} class="btn btn-sm btn-ghost"><Icon name="chevron-left" size={12} sw={2} />Settings</a><h2 class="h2">Sandbox</h2></div>
    <section class="card card-pad" aria-label="Sandbox" style="display: grid; grid-template-columns: 44px minmax(0,1fr) auto; gap: 14px; align-items: center">
      <span class="shield" style="background: {on ? 'color-mix(in srgb, var(--info) 16%, transparent)' : 'var(--raise)'}; color: {on ? 'var(--info)' : 'var(--faint)'}" aria-hidden="true"><Icon name="shield-check" size={22} sw={1.8} /></span>
      <span><span class="h2" style="display: block">Sandbox this instance</span><span class="small muted" style="display: block">Mods run in an isolated jail and can only reach the folders below. Uses bubblewrap 0.9.0 on this system.</span></span>
      <Toggle bind:on label="Sandbox this instance" />
    </section>
    <section class="sec" class:off-all={!on} aria-label="Access">
      <div style="display: flex; justify-content: space-between; align-items: center"><h2 class="h3">What the game can reach</h2><Seg options={PRESETS} bind:value={preset} label="Access preset" /></div>
      <div class="list">
        {#each rules as r (r.name)}
          <div class="row acc">
            <span style="color: var(--muted); display: grid; place-items: center" aria-hidden="true"><Icon name={r.icon} size={16} /></span>
            <span style="min-width: 0"><span class="trunc" style="display: block">{r.name}</span><span class="trunc small faint mono" style="display: block">{r.path}</span></span>
            <button class="select" style="height: 28px"><span class="lvl {LEVELS[r.level][1]}">{LEVELS[r.level][0]}</span><Icon name="chevron-down" size={12} sw={2} /></button>
          </div>
        {/each}
      </div>
      <button class="btn" style="align-self: flex-start">Allow another folder</button>
    </section>
    <section class="card" aria-label="Mod check">
      <Setting title="Check jars before launch" desc="47 of 48 mods match a known Modrinth or CurseForge file. 1 is unknown."><Toggle bind:on={checkJars} label="Check jars before launch" /></Setting>
      <div class="divider"></div>
      <Setting title="Scan for known malware" desc="Looks for signatures like fractureiser. Signatures updated 2 days ago."><Toggle bind:on={scan} label="Scan for known malware" /></Setting>
    </section>
  </div>
</div>

<style>
  .acc{grid-template-columns:20px minmax(0,1fr) 128px;height:52px}
  .lvl{font-size:12px;font-family:var(--mono)}
  .l-rw{color:var(--accent)} .l-ro{color:var(--info)} .l-no{color:var(--faint)}
  .off-all{opacity:.4;pointer-events:none}
  .shield{width:44px;height:44px;border-radius:10px;display:grid;place-items:center;flex:0 0 44px}
</style>

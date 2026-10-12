<script lang="ts">
  import { api } from '$lib/api';
  import type { Instance } from '$lib/api/types';
  import { query } from '$lib/query.svelte';
  import { href } from '$lib/router.svelte';
  import { move, repair, trash } from '$lib/instanceActions';
  import PromptDialog from '$lib/ui/PromptDialog.svelte';
  import { prefs } from '$lib/prefs.svelte';
  import { revs } from '$lib/revs.svelte';
  import { ON_GAME_START } from '$lib/settings/registry';
  import { session } from '$lib/session.svelte';
  import Seg from '$lib/ui/Seg.svelte';
  import Select from '$lib/ui/Select.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';
  import ToggleRow from '$lib/ui/ToggleRow.svelte';

  interface Props { instance: Instance }
  let { instance: i }: Props = $props();

  // Only what is turned on in Syncing shows here, in the same order.
  const sync = query(() => api.sync.groups());
  const syncable = $derived(sync.data ? [...sync.data.files, ...sync.data.folders, ...sync.data.mods, ...sync.data.custom].filter((e) => e.enabled && e.available) : []);
  let joined = $state<Record<string, boolean>>({ options: true, servers: true, hotbar: true, resourcepacks: true, xaero: true });

  // Java, memory, window, performance and commands are the instance's own (saved
  // through Go); syncing and quick play stay local until their milestones land.
  // svelte-ignore state_referenced_locally
  let st = $state($state.snapshot(i.settings));
  const info = query(() => api.app.info());
  const runtimes = query(() => api.java.runtimes());
  const groups = query(() => api.instances.groups());
  const javaOptions = $derived([{ value: '', label: `Automatic (${i.javaAuto})` }, ...(runtimes.data ?? []).map((r) => ({ value: r.path, label: r.name }))]);
  // "6 GB suits 48 mods": about 2 GB plus 85 MB a mod, never more than half the PC's memory.
  const suggestGb = $derived(i.counts.mods > 0 && prefs.value?.java.suggestMemory && info.data?.memoryMb
    ? Math.min(Math.max(2, Math.round((2048 + i.counts.mods * 85) / 1024)), Math.floor(info.data.memoryMb / 2048)) : 0);
  const memoryDesc = $derived(info.data?.memoryMb ? `This PC has ${Math.round(info.data.memoryMb / 1024)} GB.${suggestGb ? ` ${suggestGb} GB suits ${i.counts.mods} mods.` : ''}` : undefined);
  const groupOptions = $derived([{ value: '', label: 'None' }, ...(groups.data ?? []).map((g) => ({ value: g.name, label: g.name })), { value: '\u0000new', label: 'New group' }]);
  let naming = $state(false);
  const onStart = $derived(st.window.onGameStart || prefs.value?.general.onGameStart || 'hide');
  const num = (v: string) => Math.max(0, Math.round(Number(v)) || 0);

  async function save() {
    try {
      await api.instances.updateSettings(i.id, $state.snapshot(st));
      revs.bump('instances');
    } catch (e) {
      session.fail('Instance settings', e);
    }
  }
  function change(fn: () => void) {
    fn();
    void save();
  }
  let quickPlay = $state('none');
  let sharedKeys = $state(true);
  const sub = (s: string) => href({ name: 'instance', id: i.id, tab: 'settings', sub: s });

  async function edit(fn: () => Promise<unknown>, what: string) {
    try {
      await fn();
      revs.bump('instances');
    } catch (e) {
      session.fail(what, e);
    }
  }
  function setGroup(v: string) {
    if (v === '\u0000new') naming = true;
    else void edit(() => api.instances.setGroup(i.id, v), 'Group');
  }
  const tagsText = $derived(i.tags.join(', '));
</script>

<div class="scroll">
  <div class="page set-grid" style="gap: 24px; padding-top: 18px">
    <section class="sec" aria-label="Java and memory">
      <div class="sec-h"><h2 class="h3">Java and memory</h2><button class="ov" onclick={() => change(() => (st.java.override = !st.java.override))} aria-pressed={st.java.override}><span>Override global settings</span><span class="tg" class:on={st.java.override} aria-hidden="true"><span></span></span></button></div>
      <div class="card" class:inherited={!st.java.override}>
        <Setting title="Java runtime" desc="Automatic picks the version this Minecraft needs."><Select value={st.java.javaPath} options={javaOptions} label="Java runtime" onchange={(v) => change(() => (st.java.javaPath = v))} /></Setting>
        <div class="divider"></div>
        <Setting title="Memory" desc={memoryDesc}><span style="display: grid; grid-template-columns: 1fr 1fr; gap: 8px"><label class="input"><span class="sr">Minimum</span><input value={st.java.memoryMinMb} class="tnum" inputmode="numeric" onchange={(e) => change(() => (st.java.memoryMinMb = num(e.currentTarget.value)))} /><span class="suffix">MB min</span></label><label class="input"><span class="sr">Maximum</span><input value={st.java.memoryMaxMb} class="tnum" inputmode="numeric" onchange={(e) => change(() => (st.java.memoryMaxMb = num(e.currentTarget.value)))} /><span class="suffix">MB max</span></label></span></Setting>
        <div class="divider"></div>
        <div style="padding: 10px 16px 14px; display: flex; flex-direction: column; gap: 6px"><span class="setting-title">JVM arguments</span><textarea class="textarea" rows="2" aria-label="JVM arguments" value={st.java.jvmArgs} onchange={(e) => change(() => (st.java.jvmArgs = e.currentTarget.value))}></textarea></div>
      </div>
    </section>
    <section class="sec" aria-label="Game window">
      <div class="sec-h"><h2 class="h3">Game window</h2></div>
      <div class="card">
        <Setting title="Size"><span style="display: grid; grid-template-columns: minmax(0,1fr) auto minmax(0,1fr); gap: 8px; align-items: center"><label class="input"><span class="sr">Width</span><input value={st.window.width || ''} placeholder="Default" class="tnum" inputmode="numeric" onchange={(e) => change(() => (st.window.width = num(e.currentTarget.value)))} /></label><span class="faint">×</span><label class="input"><span class="sr">Height</span><input value={st.window.height || ''} placeholder="Default" class="tnum" inputmode="numeric" onchange={(e) => change(() => (st.window.height = num(e.currentTarget.value)))} /></label></span></Setting>
        <div class="divider"></div>
        <Setting title="Start fullscreen"><Toggle on={st.window.fullscreen} label="Start fullscreen" onchange={(v) => change(() => (st.window.fullscreen = v))} /></Setting>
        <div class="divider"></div>
        <Setting title="When the game starts"><Seg options={ON_GAME_START} value={onStart} label="When the game starts" onchange={(v) => change(() => (st.window.onGameStart = v))} /></Setting>
        <div class="divider"></div>
        <Setting title="Open straight into" desc="Skips the title screen."><Select bind:value={quickPlay} options={[{ value: 'none', label: 'Nothing' }, { value: 'northwatch', label: 'World: Northwatch' }, { value: 'smp', label: 'Server: Friends SMP' }]} label="Open straight into" /></Setting>
      </div>
    </section>
    <section class="sec" aria-label="Syncing">
      <div class="sec-h"><h2 class="h3">Syncing</h2><a href={href({ name: 'syncing' })} class="btn btn-sm btn-ghost">Sync settings</a></div>
      <div class="card">
        <Setting title="Shared keybinds" desc="Writes your 7 chosen keys into this instance on every launch. Other options stay untouched."><Toggle bind:on={sharedKeys} label="Shared keybinds" /></Setting>
        <div class="divider"></div>
        <div class="tgl-grid">
          {#each syncable as e (e.id)}<ToggleRow name={e.name || 'Custom'} hint={e.path} on={!!joined[e.id]} onchange={(v) => (joined[e.id] = v)} />{/each}
        </div>
      </div>
    </section>
    <section class="sec" aria-label="Sandbox">
      <div class="sec-h"><h2 class="h3">Sandbox</h2></div>
      <Setting title="Run in a sandbox" desc="Mods can only reach this instance's folder. Standard access rules." href={sub('sandbox')} class="card"><span class="badge badge-info">{i.sandboxed ? 'On' : 'Off'}</span></Setting>
    </section>
    <section class="sec" aria-label="Performance">
      <div class="sec-h"><h2 class="h3">Performance</h2><span class="small faint">Linux</span></div>
      <div class="card">
        <Setting title="Use the dedicated GPU" desc={i.gpu || 'This PC has one GPU.'}><Toggle on={st.performance.dedicatedGpu} label="Use the dedicated GPU" onchange={(v) => change(() => (st.performance.dedicatedGpu = v))} /></Setting>
        <div class="divider"></div>
        <Setting title="Feral GameMode" desc="Asks the system for CPU and GPU performance while playing."><Toggle on={st.performance.gameMode} label="Feral GameMode" onchange={(v) => change(() => (st.performance.gameMode = v))} /></Setting>
        <div class="divider"></div>
        <Setting title="MangoHud overlay" desc="Shows FPS, frame times and temperatures."><Toggle on={st.performance.mangoHud} label="MangoHud overlay" onchange={(v) => change(() => (st.performance.mangoHud = v))} /></Setting>
      </div>
    </section>
    <section class="sec" aria-label="About this instance">
      <div class="sec-h"><h2 class="h3">About this instance</h2></div>
      <div class="card">
        <Setting title="Group"><Select value={i.group ?? ''} options={groupOptions} label="Group" onchange={setGroup} /></Setting>
        <div class="divider"></div>
        <Setting title="Tags" desc="Separate tags with commas."><label class="input"><span class="sr">Tags</span><input value={tagsText} placeholder="for example: survival, friends" onchange={(e) => edit(() => api.instances.setTags(i.id, e.currentTarget.value.split(',')), 'Tags')} /></label></Setting>
        <div class="divider"></div>
        <div style="padding: 10px 16px 14px; display: flex; flex-direction: column; gap: 6px"><span class="setting-title">Notes</span><textarea class="textarea" rows="3" aria-label="Notes" maxlength="4000" value={i.notes} onchange={(e) => edit(() => api.instances.setNotes(i.id, e.currentTarget.value), 'Notes')}></textarea></div>
      </div>
    </section>
    <section class="sec" aria-label="Advanced">
      <div class="sec-h"><h2 class="h3">Advanced</h2><a href={sub('commands')} class="btn btn-sm btn-ghost">Commands and variables</a></div>
      <div class="card">
        <Setting title="Repair game files" desc="Re-checks libraries, assets and the loader against their hashes."><button class="btn btn-sm" onclick={() => repair(i)}>Repair</button></Setting>
        <div class="divider"></div>
        <Setting title="Game folder" desc={i.folder}><span style="display: flex; gap: 6px; justify-content: flex-end">{#if i.moved}<button class="btn btn-sm btn-ghost" onclick={() => move(i, true)}>Default location</button>{/if}<button class="btn btn-sm" onclick={() => move(i)}>Move</button></span></Setting>
        <div class="divider"></div>
        <Setting title="Move to trash" desc="You can undo this for 30 days."><button class="btn btn-sm btn-danger" onclick={() => trash(i)}>Move to trash</button></Setting>
      </div>
    </section>
  </div>
</div>

{#if naming}
  <PromptDialog title="New group" label="Name" help="The instance moves into it." action="Create group" maxlength={32}
    submit={(v) => edit(() => api.instances.setGroup(i.id, v), 'Group')} onclose={() => (naming = false)} />
{/if}

<style>
  .ov{display:inline-flex;align-items:center;gap:8px;font-size:12px;color:var(--faint);border:0;background:transparent;padding:0}
  .inherited{opacity:.55}
</style>

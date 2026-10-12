<script lang="ts">
  import { api } from '$lib/api';
  import type { Instance } from '$lib/api/types';
  import { query } from '$lib/query.svelte';
  import { go, href } from '$lib/router.svelte';
  import { prefs } from '$lib/prefs.svelte';
  import { revs } from '$lib/revs.svelte';
  import { tasks } from '$lib/tasks.svelte';
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

  // Java, memory and window settings are the instance's own (saved through Go); the
  // sections below them stay local until syncing, quick play and performance land.
  // svelte-ignore state_referenced_locally
  let st = $state($state.snapshot(i.settings));
  const info = query(() => api.app.info());
  const runtimes = query(() => api.java.runtimes());
  const javaOptions = $derived([{ value: '', label: 'Automatic' }, ...(runtimes.data ?? []).map((r) => ({ value: r.path, label: r.name }))]);
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
  let gpu = $state(true);
  let gamemode = $state(true);
  let mangohud = $state(false);
  const sub = (s: string) => href({ name: 'instance', id: i.id, tab: 'settings', sub: s });

  async function repair() {
    try {
      const job = await api.launch.repair(i.id);
      if (job) await tasks.load();
      else session.notify({ subject: i.name, text: 'All game files are intact.' });
    } catch (e) {
      session.fail('Repair', e);
    }
  }

  function trash() {
    go({ name: 'library' });
    session.notify({ tone: 'trash', subject: i.name, text: 'to the trash', undo: () => go({ name: 'instance', id: i.id, tab: 'overview' }) });
  }
</script>

<div class="scroll">
  <div class="page set-grid" style="gap: 24px; padding-top: 18px">
    <section class="sec" aria-label="Java and memory">
      <div class="sec-h"><h2 class="h3">Java and memory</h2><button class="ov" onclick={() => change(() => (st.java.override = !st.java.override))} aria-pressed={st.java.override}><span>Override global settings</span><span class="tg" class:on={st.java.override} aria-hidden="true"><span></span></span></button></div>
      <div class="card" class:inherited={!st.java.override}>
        <Setting title="Java runtime" desc="Automatic picks the version this Minecraft needs."><Select value={st.java.javaPath} options={javaOptions} label="Java runtime" onchange={(v) => change(() => (st.java.javaPath = v))} /></Setting>
        <div class="divider"></div>
        <Setting title="Memory" desc={info.data?.memoryMb ? `This PC has ${Math.round(info.data.memoryMb / 1024)} GB.` : undefined}><span style="display: grid; grid-template-columns: 1fr 1fr; gap: 8px"><label class="input"><span class="sr">Minimum</span><input value={st.java.memoryMinMb} class="tnum" inputmode="numeric" onchange={(e) => change(() => (st.java.memoryMinMb = num(e.currentTarget.value)))} /><span class="suffix">MB min</span></label><label class="input"><span class="sr">Maximum</span><input value={st.java.memoryMaxMb} class="tnum" inputmode="numeric" onchange={(e) => change(() => (st.java.memoryMaxMb = num(e.currentTarget.value)))} /><span class="suffix">MB max</span></label></span></Setting>
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
          {#each syncable as e (e.id)}<ToggleRow name={e.name || e.path} hint={e.name ? e.path : undefined} on={!!joined[e.id]} onchange={(v) => (joined[e.id] = v)} />{/each}
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
        <Setting title="Use the dedicated GPU" desc="NVIDIA GeForce RTX 3070 Laptop GPU"><Toggle bind:on={gpu} label="Use the dedicated GPU" /></Setting>
        <div class="divider"></div>
        <Setting title="Feral GameMode" desc="Asks the system for CPU and GPU performance while playing."><Toggle bind:on={gamemode} label="Feral GameMode" /></Setting>
        <div class="divider"></div>
        <Setting title="MangoHud overlay" desc="Shows FPS, frame times and temperatures."><Toggle bind:on={mangohud} label="MangoHud overlay" /></Setting>
      </div>
    </section>
    <section class="sec" aria-label="Advanced">
      <div class="sec-h"><h2 class="h3">Advanced</h2><a href={sub('commands')} class="btn btn-sm btn-ghost">Commands and variables</a></div>
      <div class="card">
        <Setting title="Repair game files" desc="Re-checks libraries, assets and the loader against their hashes."><button class="btn btn-sm" onclick={repair}>Repair</button></Setting>
        <div class="divider"></div>
        <Setting title="Move to trash" desc="You can undo this for 30 days."><button class="btn btn-sm btn-danger" onclick={trash}>Move to trash</button></Setting>
      </div>
    </section>
  </div>
</div>

<style>
  .ov{display:inline-flex;align-items:center;gap:8px;font-size:12px;color:var(--faint);border:0;background:transparent;padding:0}
  .inherited{opacity:.55}
</style>

<script lang="ts">
  import { api } from '$lib/api';
  import type { Instance } from '$lib/api/types';
  import { query } from '$lib/query.svelte';
  import { href } from '$lib/router.svelte';
  import { move, repair, trash } from '$lib/instanceActions';
  import { plural } from '$lib/format';
  import { suggestMemoryGb } from '$lib/look';
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
  const sharedKeys = query(() => api.sync.keybinds());
  const syncable = $derived(sync.data ? [...sync.data.files, ...sync.data.folders, ...sync.data.mods, ...sync.data.custom].filter((e) => e.enabled && e.available) : []);
  // Joining moves the instance's own items into the shared copy; leaving keeps a copy.
  const join = (target: string, on: boolean) =>
    edit(async () => {
      await api.instances.setSync(i.id, target, on);
      revs.bump('sync');
    }, 'Syncing');

  // Java, memory, window (with "Open straight into"), performance and commands are
  // the instance's own (saved through Go).
  // svelte-ignore state_referenced_locally
  let st = $state($state.snapshot(i.settings));
  const info = query(() => api.app.info());
  const runtimes = query(() => api.java.runtimes());
  const groups = query(() => api.instances.groups());
  const javaOptions = $derived([{ value: '', label: `Automatic (${i.javaAuto})` }, ...(runtimes.data ?? []).map((r) => ({ value: r.path, label: r.name }))]);
  const suggestGb = $derived(prefs.value?.java.suggestMemory ? suggestMemoryGb(i.counts.mods, info.data?.memoryMb ?? 0) : 0);
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
  // "Open straight into": the instance's own worlds and servers.
  const worlds = query(() => api.worlds.list(i.id));
  const servers = query(() => api.servers.list(i.id));
  const into = $derived(st.window.openInto?.kind ? `${st.window.openInto.kind}:${st.window.openInto.target}` : 'none');
  const intoOptions = $derived.by(() => {
    const opts = [{ value: 'none', label: 'Nothing' }];
    for (const w of worlds.data ?? []) opts.push({ value: `world:${w.id}`, label: `World: ${w.name}` });
    for (const s of servers.data ?? []) {
      const e = api.servers.entry(i.id, s.id);
      if (e) opts.push({ value: `server:${e.address}`, label: `Server: ${s.name}` });
    }
    // A world or server that is gone keeps its entry, so the select never shows a blank.
    const t = st.window.openInto;
    if (t?.kind && !opts.some((o) => o.value === into)) opts.push({ value: into, label: `${t.kind === 'world' ? 'World' : 'Server'}: ${t.target}` });
    return opts;
  });
  function setInto(v: string) {
    const at = v.indexOf(':');
    const kind = v === 'none' ? '' : (v.slice(0, at) as 'world' | 'server');
    change(() => (st.window.openInto = { kind, target: kind ? v.slice(at + 1) : '' }));
  }
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
        <Setting title="Open straight into" desc="Skips the title screen."><Select value={into} options={intoOptions} label="Open straight into" onchange={setInto} /></Setting>
      </div>
    </section>
    <section class="sec" aria-label="Syncing">
      <div class="sec-h"><h2 class="h3">Syncing</h2><a href={href({ name: 'syncing' })} class="btn btn-sm btn-ghost">Sync settings</a></div>
      <div class="card">
        <Setting title="Shared keybinds" desc="Writes your {plural(sharedKeys.data?.length ?? 0, 'chosen key')} into this instance on every launch. Other options stay untouched."><Toggle on={i.sync.keys} label="Shared keybinds" onchange={(v) => edit(() => api.instances.setSharedKeys(i.id, v), 'Shared keybinds')} /></Setting>
        <div class="divider"></div>
        <div class="tgl-grid">
          {#each syncable as e (e.id)}<ToggleRow name={e.name || 'Custom'} hint={e.path} on={i.sync.join.includes(e.id)} onchange={(v) => join(e.id, v)} />{/each}
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

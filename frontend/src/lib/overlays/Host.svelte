<script lang="ts">
  import serverIcon from '$assets/server-icon.png';
  import { api } from '../api';
  import { gb, mb } from '../format';
  import type { HostOptions, HostWorld } from '../api/host';
  import { isPicture } from '../look';
  import { query } from '../query.svelte';
  import { session } from '../session.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Icon from '../ui/Icon.svelte';
  import NumberField from '../ui/NumberField.svelte';
  import Seg from '../ui/Seg.svelte';
  import ServerPing from '../ui/ServerPing.svelte';
  import Setting from '../ui/Setting.svelte';
  import Toggle from '../ui/Toggle.svelte';
  import HostMods from './HostMods.svelte';
  import HostProps from './HostProps.svelte';

  /** Set up a local server for an instance (DlgHost.dc.html): on the left how players see
      it (the server-list row, the world), on the right how it runs. */
  interface Props { instanceId: string }
  let { instanceId }: Props = $props();
  const inst = query(() => api.instances.get(instanceId));
  const setup = query(() => api.host.setup(instanceId));
  let modsRev = $state(0);
  const mods = query(() => (void modsRev, api.host.mods(instanceId)));
  const NEW_WORLD: HostWorld = { folder: '', name: 'New world', meta: 'Random seed, created on start', size: '', art: { background: 'repeating-linear-gradient(45deg,var(--line-2) 0 4px,transparent 4px 8px)' } };
  const rows = $derived(setup.data ? [...setup.data.worlds, NEW_WORLD] : []);
  const DIFFICULTIES = ['peaceful', 'easy', 'normal', 'hard'].map((value) => ({ value, label: value[0]!.toUpperCase() + value.slice(1) }));

  let world = $state('');
  let seed = $state('');
  let motd = $state('');
  let icon = $state('');
  let view = $state('');
  let memory = $state('');
  let players = $state('');
  let port = $state('');
  let difficulty = $state('normal');
  let lan = $state(true);
  let offline = $state(false);
  let sandbox = $state(false);
  let managing = $state(false);
  let editing = $state(false);
  // The last options used (or Go's defaults) fill the form once.
  let filled = false;
  $effect(() => {
    const s = setup.data;
    if (!s || filled) return;
    filled = true;
    const o = s.options;
    ({ world, lan, offline, motd } = o);
    seed = o.seed ?? '';
    icon = o.icon ?? '';
    sandbox = o.sandbox || s.sandboxLocked;
    [view, memory, players, port] = [o.viewDistance, gb(o.memoryMB), o.maxPlayers, o.port].map(String) as [string, string, string, string];
    difficulty = o.difficulty || s.difficulty[o.world] || 'normal';
  });
  // A hardcore world is always Hard; otherwise picking a world shows its own difficulty
  // until the switch is used.
  let pickedDifficulty = false;
  const hardcore = $derived(!!setup.data?.hardcore[world]);
  function pickWorld(folder: string) {
    world = folder;
    if (!pickedDifficulty) difficulty = setup.data?.difficulty[folder] || difficulty;
  }

  const modsText = $derived.by(() => {
    const m = mods.data;
    if (!m) return '';
    if (m.length === 0) return 'This instance has no mods.';
    const on = m.filter((x) => x.on).length;
    const out = m.length - on;
    return out === 0 ? `All ${m.length} go on the server.` : `${on} of ${m.length} go on the server. ${out} ${out === 1 ? 'is' : 'are'} left out.`;
  });

  async function chooseIcon() {
    try {
      icon = (await api.instances.chooseIcon()) || icon;
    } catch (e) {
      session.fail('Server icon', e);
    }
  }

  const num = (s: string) => (/^\d+$/.test(s.trim()) ? Number(s.trim()) : NaN);
  function start() {
    const o: HostOptions = {
      world, port: num(port), maxPlayers: num(players), memoryMB: mb(num(memory)), viewDistance: num(view),
      motd: motd.trim(), difficulty: hardcore ? 'hard' : difficulty, icon, lan, offline, sandbox, ...(world === '' && seed.trim() ? { seed: seed.trim() } : {}),
    };
    if ([o.port, o.maxPlayers, o.memoryMB, o.viewDistance].some(Number.isNaN)) {
      session.notify({ tone: 'error', subject: "Couldn't start the server", text: 'Render distance, memory, max players and port are whole numbers.' });
      return;
    }
    session.startHost(instanceId, o);
  }
  const openFolder = () => api.host.openFolder(instanceId).catch((e) => session.fail('Open folder', e));
</script>

<Dialog title="Host {inst.data?.name ?? ''}" width={880} onclose={() => session.close()}>
  {#snippet head()}
    <span style="display: flex; align-items: center; gap: 8px; flex: 1; min-width: 0">
      <span style="min-width: 0; margin-right: auto"><span class="h2 trunc" style="display: block">Host {inst.data?.name ?? ''}</span><span class="small faint">Runs a server on this computer with the same version, loader and mods.</span></span>
      <button class="btn-bare" aria-label="Open server folder" title="Open server folder" onclick={openFolder}><Icon name="folder" size={15} sw={1.8} /></button>
    </span>
  {/snippet}
  <div class="cols">
    <div class="col">
      <div class="field"><span class="field-label">In the server list</span>
        <div class="mcrow">
          <button class="svico" aria-label="Change server icon" title="Change server icon" onclick={chooseIcon}><img src={isPicture(icon) ? `/tex/icon/${icon}.png` : serverIcon} alt="" draggable="false" /></button>
          <span style="display: flex; flex-direction: column; gap: 6px; min-width: 0"><span style="font-weight: 600">Minecraft Server</span><label class="input"><span class="sr">MOTD</span><input bind:value={motd} placeholder="MOTD" maxlength="59" /></label></span>
          <span style="display: flex; flex-direction: column; align-items: flex-end; gap: 6px"><ServerPing ms={0} bare /><span class="small faint tnum">0/{num(players) || 0}</span></span>
        </div>
        <span class="field-help">Click the icon to change it{#if icon}, or <button class="tlink" onclick={() => (icon = '')}>use the Quartz icon</button>{/if}. An empty MOTD uses the instance name.</span>
      </div>
      <div class="field wgrow"><span class="field-label">World</span>
        <div class="list wbox" role="radiogroup" aria-label="World">
          <div class="wscroll">
            {#if setup.data}
              {#each rows as w (w.folder)}
                <button class="wr" class:on={world === w.folder} onclick={() => pickWorld(w.folder)} role="radio" aria-checked={world === w.folder}><span class="radio" class:on={world === w.folder} aria-hidden="true"></span><span style="width: 32px; height: 32px; border-radius: 6px; background: {w.art.background}" aria-hidden="true"></span><span style="min-width: 0"><span class="trunc" style="display: block">{w.name}</span><span class="trunc small faint" style="display: block">{w.meta}</span></span><span class="small faint">{w.size}</span></button>
              {/each}
              {#if world === ''}<div class="seedrow"><label class="input" style="height: 30px"><span class="sr">Seed</span><input class="mono" bind:value={seed} placeholder="Seed (random when empty)" maxlength="32" /></label></div>{/if}
            {:else}
              {#each [44, 36, 30] as width (width)}
                <div class="wr" aria-hidden="true"><span class="sk" style="width: 16px; height: 16px; border-radius: 50%"></span><span class="sk" style="width: 32px; height: 32px; border-radius: 6px"></span><span><span class="sk" style="display: block; height: 12px; width: {width}%"></span><span class="sk" style="display: block; height: 10px; width: {width + 12}%; margin-top: 6px"></span></span><span></span></div>
              {/each}
            {/if}
          </div>
          <span class="field-help wnote">The server plays the world in place. A backup is made first.</span>
        </div>
      </div>
    </div>
    <div class="col">
      <div class="nums">
        <NumberField label="Render Distance" bind:value={view} min={2} max={32} />
        <NumberField label="Server Memory" bind:value={memory} min={1} max={Math.max(1, gb(setup.data?.maxMemoryMB ?? 1024))} unit="GB" />
        <NumberField label="Max Players" bind:value={players} min={1} max={999} />
        <NumberField label="Server Port" bind:value={port} min={1024} max={65535} mono />
      </div>
      <div class="field"><span class="field-label">Difficulty</span>
        <Seg options={DIFFICULTIES.map((d) => ({ ...d, disabled: hardcore && d.value !== 'hard' }))} value={hardcore ? 'hard' : difficulty} label="Difficulty" fill center onchange={(v) => { difficulty = v; pickedDifficulty = true; }} />
      </div>
      <div class="card">
        <div class="setting" style="min-height: 48px"><span><span class="setting-title" style="display: block">Mods</span><span class="setting-desc" style="display: block">{#if mods.data}{modsText}{:else}<span class="sk" style="display: inline-block; height: 10px; width: 70%" aria-hidden="true"></span>{/if}</span></span><button class="btn btn-sm btn-w" disabled={!mods.data?.length} onclick={() => (managing = true)}>Manage</button></div>
        <div class="divider"></div>
        <div class="setting" style="min-height: 48px"><span><span class="setting-title" style="display: block">server.properties</span><span class="setting-desc" style="display: block">View rules, whitelist, view distance and the rest.</span></span><button class="btn btn-sm btn-w" onclick={() => (editing = true)}>Edit</button></div>
        <div class="divider"></div>
        <Setting title="Show on your local network" desc="Friends on the same Wi-Fi see it in their server list." style="min-height: 48px"><Toggle bind:on={lan} label="Show on your local network" /></Setting>
        <div class="divider"></div>
        <Setting title="Allow local accounts" desc="Lets players join without a Microsoft account. Less safe." style="min-height: 48px"><Toggle bind:on={offline} label="Allow local accounts" /></Setting>
        {#if setup.data?.canSandbox || setup.data?.sandboxLocked}
          <div class="divider"></div>
          <Setting title="Sandbox" desc={setup.data.sandboxLocked ? 'This instance is sandboxed, so its server is too.' : 'Runs the server where it can only reach its own folder and the world.'} style="min-height: 48px">
            {#if setup.data.sandboxLocked}<Toggle on disabled label="Sandbox" />{:else}<Toggle bind:on={sandbox} label="Sandbox" />{/if}
          </Setting>
        {/if}
      </div>
    </div>
  </div>
  {#snippet foot()}<button class="btn" onclick={() => session.close()}>Cancel</button><button class="btn btn-accent" onclick={start} disabled={!setup.data}>Start server</button>{/snippet}
</Dialog>

{#if managing && mods.data}
  <HostMods {instanceId} mods={mods.data} onclose={() => (managing = false)} onsaved={() => modsRev++} />
{/if}
{#if editing}
  <HostProps {instanceId} name={inst.data?.name ?? ''} onclose={() => (editing = false)} />
{/if}

<style>
  .cols{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:18px;align-items:stretch}
  .col{display:flex;flex-direction:column;gap:14px;min-width:0}
  .nums{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px}
  /* The server-list row, in the dialog's own colours. */
  .mcrow{display:grid;grid-template-columns:64px minmax(0,1fr) auto;gap:14px;align-items:center;padding:12px;border:1px solid var(--line);border-radius:var(--radius-lg);background:var(--panel)}
  .svico{width:64px;height:64px;padding:0;border:1px solid var(--line-2);border-radius:4px;background:var(--raise);overflow:hidden}
  .svico img{width:100%;height:100%;display:block;image-rendering:pixelated}
  /* The world box grows to the foot of the right column; its rows scroll above the note. */
  .wgrow{flex:1 1 auto;min-height:0}
  .wbox{flex:1 1 auto;min-height:192px;display:flex;flex-direction:column}
  .wscroll{flex:1 1 0;min-height:0;overflow-y:auto}
  .wnote{padding:10px 12px}
  .wr{display:grid;grid-template-columns:16px 32px minmax(0,1fr) auto;gap:12px;align-items:center;height:48px;padding:0 12px;border:0;background:transparent;width:100%;text-align:left;color:var(--text);border-top:1px solid var(--line)}
  .wr:first-child{border-top:0}
  .wr.on{background:var(--raise)}
  .seedrow{padding:0 12px 10px 40px;background:var(--raise)}
  @media (max-width:900px){.cols{grid-template-columns:minmax(0,1fr)}}
</style>

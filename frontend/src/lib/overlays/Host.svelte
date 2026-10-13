<script lang="ts">
  import { api } from '../api';
  import type { HostWorld } from '../api/host';
  import { query } from '../query.svelte';
  import { go } from '../router.svelte';
  import { session } from '../session.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Toggle from '../ui/Toggle.svelte';
  import Setting from '../ui/Setting.svelte';

  /** Set up a local server for an instance (DlgHost.dc.html). */
  interface Props { instanceId: string }
  let { instanceId }: Props = $props();
  const inst = query(() => api.instances.get(instanceId));
  const setup = query(() => api.host.setup(instanceId));
  const mods = query(() => api.host.mods(instanceId));
  const NEW_WORLD: HostWorld = { folder: '', name: 'New world', meta: 'Random seed, created on start', size: '', art: { background: 'repeating-linear-gradient(45deg,var(--line-2) 0 4px,transparent 4px 8px)' } };
  const rows = $derived(setup.data ? [...setup.data.worlds, NEW_WORLD] : []);

  let world = $state('');
  let lan = $state(true);
  let offline = $state(false);
  let port = $state('');
  let players = $state('');
  let memory = $state('');
  // The last options used (or Go's defaults) fill the form once.
  let filled = false;
  $effect(() => {
    const s = setup.data;
    if (!s || filled) return;
    filled = true;
    ({ world, lan, offline } = s.options);
    port = String(s.options.port);
    players = String(s.options.maxPlayers);
    memory = String(s.options.memoryMB);
  });

  /** "31 of 48 also run on servers. 17 client-only mods are left out." */
  const modsText = $derived.by(() => {
    const m = mods.data;
    if (!m) return '';
    if (m.total === 0) return 'No mods to leave out.';
    const out = m.clientOnly.length;
    if (out === 0) return `All ${m.total} also run on servers.`;
    return `${m.server} of ${m.total} also run on servers. ${out} client-only ${out === 1 ? 'mod is' : 'mods are'} left out.`;
  });

  const num = (s: string) => (/^\d+$/.test(s.trim()) ? Number(s.trim()) : NaN);
  function start() {
    const o = { world, port: num(port), maxPlayers: num(players), memoryMB: num(memory), lan, offline };
    if ([o.port, o.maxPlayers, o.memoryMB].some(Number.isNaN)) {
      session.notify({ tone: 'error', subject: "Couldn't start the server", text: 'Port, max players and memory are whole numbers.' });
      return;
    }
    session.startHost(instanceId, o);
  }
  function review() {
    session.close();
    go({ name: 'instance', id: instanceId, tab: 'mods', sub: 'client-only' });
  }
</script>

<Dialog title="Host {inst.data?.name ?? ''}" sub="Runs a server on this computer with the same version, loader and mods." width={560} onclose={() => session.close()}>
  <div class="field"><span class="field-label">World</span>
    <div class="list" role="radiogroup" aria-label="World">
      {#if setup.data}
        {#each rows as w (w.folder)}
          <button class="wr" class:on={world === w.folder} onclick={() => (world = w.folder)} role="radio" aria-checked={world === w.folder}><span class="radio" class:on={world === w.folder} aria-hidden="true"></span><span style="width: 32px; height: 32px; border-radius: 6px; background: {w.art.background}" aria-hidden="true"></span><span style="min-width: 0"><span class="trunc" style="display: block">{w.name}</span><span class="trunc small faint" style="display: block">{w.meta}</span></span><span class="small faint">{w.size}</span></button>
        {/each}
      {:else}
        {#each [44, 36, 30] as width (width)}
          <div class="wr" aria-hidden="true"><span class="sk" style="width: 16px; height: 16px; border-radius: 50%"></span><span class="sk" style="width: 32px; height: 32px; border-radius: 6px"></span><span><span class="sk" style="display: block; height: 12px; width: {width}%"></span><span class="sk" style="display: block; height: 10px; width: {width + 12}%; margin-top: 6px"></span></span><span></span></div>
        {/each}
      {/if}
    </div>
    <span class="field-help">The server plays the world in place. A backup is made first.</span>
  </div>
  <div style="display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px">
    <label class="field"><span class="field-label">Port</span><span class="input"><input class="mono" bind:value={port} inputmode="numeric" /></span></label>
    <label class="field"><span class="field-label">Max players</span><span class="input"><input class="tnum" bind:value={players} inputmode="numeric" /></span></label>
    <label class="field"><span class="field-label">Memory</span><span class="input"><input class="tnum" bind:value={memory} inputmode="numeric" /><span class="suffix">MB</span></span></label>
  </div>
  <div class="card">
    <div class="setting" style="min-height: 48px"><span><span class="setting-title" style="display: block">Mods</span><span class="setting-desc" style="display: block">{#if mods.data}{modsText}{:else}<span class="sk" style="display: inline-block; height: 10px; width: 70%" aria-hidden="true"></span>{/if}</span></span><button class="btn btn-sm btn-ghost" onclick={review}>Review</button></div>
    <div class="divider"></div>
    <Setting title="Show on your local network" desc="Friends on the same Wi-Fi see it in their server list." style="min-height: 48px"><Toggle bind:on={lan} label="Show on your local network" /></Setting>
    <div class="divider"></div>
    <Setting title="Allow offline accounts" desc="Lets players join without a Microsoft account. Less safe." style="min-height: 48px"><Toggle bind:on={offline} label="Allow offline accounts" /></Setting>
  </div>
  {#snippet foot()}<button class="btn" onclick={() => session.close()}>Cancel</button><button class="btn btn-accent" onclick={start} disabled={!setup.data}>Start server</button>{/snippet}
</Dialog>

<style>
  .wr{display:grid;grid-template-columns:16px 32px minmax(0,1fr) auto;gap:12px;align-items:center;height:48px;padding:0 12px;border:0;background:transparent;width:100%;text-align:left;color:var(--text);border-top:1px solid var(--line)}
  .wr:first-child{border-top:0}
  .wr.on{background:var(--raise)}
</style>

<script lang="ts">
  import { api } from '../api';
  import { query } from '../query.svelte';
  import { href } from '../router.svelte';
  import { session } from '../session.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Toggle from '../ui/Toggle.svelte';
  import Setting from '../ui/Setting.svelte';

  /** Set up a local server for an instance. */
  interface Props { instanceId: string }
  let { instanceId }: Props = $props();
  const inst = query(() => api.instances.get(instanceId));
  const WORLDS = [
    { name: 'Northwatch', meta: 'Survival, played yesterday', size: '1.8 GB', bg: 'linear-gradient(175deg,#7fb7d9 0 45%,#3f7d4f 45% 72%,#2c5236 72%)' },
    { name: 'Creative Plots', meta: 'Creative, 2 weeks ago', size: '240 MB', bg: 'linear-gradient(175deg,#9ab3f0 0 45%,#6d7f9e 45% 72%,#3e4a5e 72%)' },
    { name: 'New world', meta: 'Random seed, created on start', size: '', bg: 'repeating-linear-gradient(45deg,var(--line-2) 0 4px,transparent 4px 8px)' },
  ];
  let world = $state(0);
  let lan = $state(true);
  let offline = $state(false);
  let port = $state('25565');
  let players = $state('10');
  let memory = $state('4096');
</script>

<Dialog title="Host {inst.data?.name ?? ''}" sub="Runs a server on this computer with the same version, loader and mods." width={560} onclose={() => session.close()}>
  <div class="field"><span class="field-label">World</span>
    <div class="list" role="radiogroup" aria-label="World">
      {#each WORLDS as w, i (w.name)}
        <button class="wr" class:on={world === i} onclick={() => (world = i)} role="radio" aria-checked={world === i}><span class="radio" class:on={world === i} aria-hidden="true"></span><span style="width: 32px; height: 32px; border-radius: 6px; background: {w.bg}" aria-hidden="true"></span><span style="min-width: 0"><span class="trunc" style="display: block">{w.name}</span><span class="trunc small faint" style="display: block">{w.meta}</span></span><span class="small faint">{w.size}</span></button>
      {/each}
    </div>
    <span class="field-help">The server plays the world in place. A backup is made first.</span>
  </div>
  <div style="display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px">
    <label class="field"><span class="field-label">Port</span><span class="input"><input class="mono" bind:value={port} inputmode="numeric" /></span></label>
    <label class="field"><span class="field-label">Max players</span><span class="input"><input class="tnum" bind:value={players} inputmode="numeric" /></span></label>
    <label class="field"><span class="field-label">Memory</span><span class="input"><input class="tnum" bind:value={memory} inputmode="numeric" /><span class="suffix">MB</span></span></label>
  </div>
  <div class="card">
    <Setting title="Mods" desc="31 of 48 also run on servers. 17 client-only mods are left out." style="min-height: 48px"><a href={href({ name: 'instance', id: instanceId, tab: 'mods' })} class="btn btn-sm btn-ghost" onclick={() => session.close()}>Review</a></Setting>
    <div class="divider"></div>
    <Setting title="Show on your local network" desc="Friends on the same Wi-Fi see it in their server list." style="min-height: 48px"><Toggle bind:on={lan} label="Show on your local network" /></Setting>
    <div class="divider"></div>
    <Setting title="Allow offline accounts" desc="Lets players join without a Microsoft account. Less safe." style="min-height: 48px"><Toggle bind:on={offline} label="Allow offline accounts" /></Setting>
  </div>
  {#snippet foot()}<button class="btn" onclick={() => session.close()}>Cancel</button><button class="btn btn-accent" onclick={() => session.startHost(instanceId)}>Start server</button>{/snippet}
</Dialog>

<style>
  .wr{display:grid;grid-template-columns:16px 32px minmax(0,1fr) auto;gap:12px;align-items:center;height:48px;padding:0 12px;border:0;background:transparent;width:100%;text-align:left;color:var(--text);border-top:1px solid var(--line)}
  .wr:first-child{border-top:0}
  .wr.on{background:var(--raise)}
</style>

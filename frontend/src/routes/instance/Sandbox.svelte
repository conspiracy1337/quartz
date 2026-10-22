<script lang="ts">
  import { api } from '$lib/api';
  import type { SandboxLevel, SandboxPreset, SandboxRow } from '$lib/api/sandbox';
  import type { Instance } from '$lib/api/types';
  import { query } from '$lib/query.svelte';
  import { revs } from '$lib/revs.svelte';
  import { href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import type { IconName } from '$lib/ui/icons';
  import Seg from '$lib/ui/Seg.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';

  interface Props { instance: Instance }
  let { instance: i }: Props = $props();

  const LEVELS: Record<SandboxLevel, [string, string]> = { rw: ['Read and write', 'l-rw'], ro: ['Read only', 'l-ro'], no: ['Blocked', 'l-no'], on: ['Allowed', 'l-rw'], lan: ['LAN only', 'l-ro'], remove: ['Remove', 'l-no'] };
  // Each rule's name and icon, and the description shown when Go names no path.
  const RULES: Record<string, { name: string; icon: IconName; path: string }> = {
    instance: { name: 'This instance', icon: 'folder', path: '' },
    mods: { name: 'Mods', icon: 'folder', path: '' },
    libraries: { name: 'Shared libraries and assets', icon: 'folder', path: '' },
    java: { name: 'Java runtime', icon: 'folder', path: '' },
    synced: { name: 'Synced folders', icon: 'folder', path: '' },
    internet: { name: 'Internet', icon: 'globe', path: 'Needed for servers and skins' },
    discord: { name: 'Discord status', icon: 'chat', path: 'Discord IPC socket' },
    graphics: { name: 'Graphics and audio', icon: 'gpu', path: 'GPU, PipeWire, Wayland' },
    home: { name: 'Your home folder', icon: 'house', path: '~/ and other instances' },
  };
  const PRESETS = [{ value: 'standard', label: 'Standard' }, { value: 'strict', label: 'Strict' }] as const;
  // The player's own folders pick their access; a rule's level comes from the preset.
  const FOLDER_LEVELS = [{ value: 'ro', label: 'Read only' }, { value: 'rw', label: 'Read and write' }] as const;

  const status = query(() => api.sandbox.status());
  const sb = query(() => api.sandbox.view(i.id));
  const on = $derived(sb.data?.on ?? i.sandboxed);
  const uses = $derived.by(() => {
    const s = status.data;
    if (!s) return '';
    return s.available ? ` Uses ${s.name}${s.version ? ` ${s.version}` : ''} on this system.` : ` ${s.reason}`;
  });

  const meta = (r: SandboxRow) => (r.folder ? { name: r.path.split(/[\\/]/).filter(Boolean).pop() ?? r.path, icon: 'folder' as IconName, path: '' } : RULES[r.id] ?? { name: r.id, icon: 'folder' as IconName, path: '' });

  /** Runs a change; the page reloads from Go's answer (instances:changed). */
  async function edit(what: string, fn: () => Promise<unknown>) {
    try {
      await fn();
    } catch (e) {
      session.fail(what, e);
    }
    revs.bump('instances');
  }
  const setOn = (v: boolean) => edit('Sandbox', () => api.sandbox.setOn(i.id, v));
  const setPreset = (p: SandboxPreset) => edit('Sandbox', () => api.sandbox.setPreset(i.id, p));
  const setFolder = (r: SandboxRow, l: SandboxLevel) => edit(meta(r).name, () => api.sandbox.setFolder(i.id, r.id, l));
  const addFolder = () => edit('Allow another folder', () => api.sandbox.addFolder(i.id));
</script>

<div class="scroll">
  <div class="page page-narrow" style="gap: 20px; padding-top: 18px">
    <div style="display: flex; align-items: center; gap: 8px"><a href={href({ name: 'instance', id: i.id, tab: 'settings' })} class="btn btn-sm btn-ghost"><Icon name="chevron-left" size={12} sw={2} />Settings</a><h2 class="h2">Sandbox</h2></div>
    <section class="card card-pad" aria-label="Sandbox" style="display: grid; grid-template-columns: 44px minmax(0,1fr) auto; gap: 14px; align-items: center">
      <span class="shield" style="background: {on ? 'color-mix(in srgb, var(--info) 16%, transparent)' : 'var(--raise)'}; color: {on ? 'var(--info)' : 'var(--faint)'}" aria-hidden="true"><Icon name="shield-check" size={22} sw={1.8} /></span>
      <span><span class="h2" style="display: block">Sandbox this instance</span><span class="small muted" style="display: block">Mods run in an isolated jail and can only reach the folders below.{uses}</span></span>
      <Toggle {on} label="Sandbox this instance" onchange={setOn} />
    </section>
    <section class="sec" class:off-all={!on} aria-label="Access">
      <div style="display: flex; justify-content: space-between; align-items: center"><h2 class="h3">What the game can reach</h2><Seg options={PRESETS} value={sb.data?.preset ?? 'standard'} label="Access preset" onchange={setPreset} /></div>
      <div class="list">
        {#if sb.data}
          {#each sb.data.rows as r (r.id)}
            {@const m = meta(r)}
            <div class="row acc" class:own={r.folder}>
              <span style="color: var(--muted); display: grid; place-items: center" aria-hidden="true"><Icon name={m.icon} size={16} /></span>
              <span style="min-width: 0"><span class="trunc" style="display: block">{m.name}</span><span class="trunc small faint mono" style="display: block">{r.path || m.path}</span></span>
              {#if r.folder}
                <Seg options={FOLDER_LEVELS} value={r.level === 'rw' ? 'rw' : 'ro'} label="{m.name} access" onchange={(l) => setFolder(r, l)} />
                <button class="btn btn-icon btn-sm btn-ghost" aria-label="Stop allowing {m.name}" title="Stop allowing this folder" onclick={() => setFolder(r, 'remove')}><Icon name="x" size={12} sw={2} /></button>
              {:else}
                <span class="lvl {LEVELS[r.level][1]}">{LEVELS[r.level][0]}</span>
              {/if}
            </div>
          {/each}
        {:else}
          {#each { length: 9 } as _, k (k)}<div class="row acc" aria-hidden="true"><div class="sk" style="width: 16px; height: 16px; border-radius: 4px"></div><div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: 35%"></div><div class="sk" style="height: 10px; width: 50%"></div></div><div class="sk" style="height: 10px; width: 72px"></div></div>{/each}
        {/if}
      </div>
      <button class="btn" style="align-self: flex-start" onclick={addFolder}>Allow another folder</button>
    </section>
  </div>
</div>

<style>
  .acc{grid-template-columns:20px minmax(0,1fr) 128px;height:52px}
  /* The player's folders: their access choice and a remove button, on the rules' right edge. */
  .acc.own{grid-template-columns:20px minmax(0,1fr) auto 28px}
  .lvl{font-size:12px;font-family:var(--mono);justify-self:end}
  .l-rw{color:var(--accent)} .l-ro{color:var(--info)} .l-no{color:var(--faint)}
  .off-all{opacity:.4;pointer-events:none}
  .shield{width:44px;height:44px;border-radius:10px;display:grid;place-items:center;flex:0 0 44px}
</style>

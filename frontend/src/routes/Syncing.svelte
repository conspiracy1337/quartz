<script lang="ts">
  import { api } from '$lib/api';
  import type { KeyChoice, SyncEntry } from '$lib/api/types';
  import { captureKey } from '$lib/keycapture';
  import { query } from '$lib/query.svelte';
  import { revs } from '$lib/revs.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Menu from '$lib/ui/Menu.svelte';
  import PageHeader from '$lib/ui/PageHeader.svelte';
  import ToggleRow from '$lib/ui/ToggleRow.svelte';
  import Empty from '$lib/ui/Empty.svelte';

  // Keyed (instance settings and Overview share them), so a second visit shows them at once.
  const groups = query(() => api.sync.groups(), 'sync.groups');
  const keys = query(() => api.sync.keybinds(), 'sync.keybinds');
  let custom = $state('');

  const users = (e: SyncEntry, total: number) => (e.enabled && e.available ? `${e.users}/${total}` : '—');
  const sections = [
    { key: 'files', title: 'Files', note: 'Newest copy wins' },
    { key: 'folders', title: 'Folders', note: 'Linked into each instance' },
    { key: 'mods', title: 'Mod data', note: 'Greyed out mods are in no instance' },
  ] as const;

  /** Runs a change, then refreshes the page and the instances that join (their counts change). */
  async function edit(what: string, fn: () => Promise<unknown>) {
    try {
      await fn();
    } catch (e) {
      session.fail(what, e);
    }
    revs.bump('sync');
    revs.bump('instances');
  }
  const toggle = (e: SyncEntry, on: boolean) => edit(e.name || e.path, () => api.sync.setEnabled(e.id, on));
  async function addCustom(dir: boolean) {
    const path = custom.trim();
    if (!path) return;
    await edit(path, async () => {
      await api.sync.addCustom(path, dir);
      custom = '';
    });
  }

  // "Add key": the keybinds the instances have, by game or mod, under the button.
  let picker = $state<{ x: number; y: number } | null>(null);
  let choices = $state.raw<KeyChoice[] | null>(null);
  const byCategory = $derived.by(() => {
    const out = new Map<string, KeyChoice[]>();
    for (const c of choices ?? []) if (!c.shared) out.set(c.category, [...(out.get(c.category) ?? []), c]);
    return [...out];
  });
  async function openPicker(e: MouseEvent) {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    choices = null;
    picker = { x: r.left, y: r.bottom + 4 };
    try {
      choices = await api.sync.keyCatalog();
    } catch (err) {
      picker = null;
      session.fail('Add key', err);
    }
  }
  function addKey(id: string) {
    picker = null;
    void edit('Add key', () => api.sync.addKey(id));
  }

  // A key button turns into "Press a key…" until the next key or mouse button.
  let capturing = $state<{ id: string; cancel: () => void } | null>(null);
  async function capture(id: string) {
    capturing?.cancel();
    const c = captureKey();
    capturing = { id, cancel: c.cancel };
    const code = await c.done;
    if (capturing?.id === id) capturing = null;
    if (code) void edit('Shared keybinds', () => api.sync.setKey(id, code));
  }
</script>

<PageHeader title="Syncing">
  {#snippet actions()}<button class="btn" onclick={() => api.sync.openFolder().catch((e) => session.fail('Open synced folder', e))}><Icon name="folder" size={14} sw={2} />Open synced folder</button>{/snippet}
</PageHeader>
<div class="scroll">
  <div class="page" style="gap: 24px; padding-top: 18px">
    <p class="muted" style="margin: 0; max-width: 80ch">Turn on what can be shared here, then pick which instances join in their own settings. Folders are linked into each instance; files keep the newest copy. The number shows how many instances use each one.</p>
    {#if groups.data}
      {@const g = groups.data}
      <section class="sec" aria-label="Shared keybinds">
        <div class="sec-h"><h2 class="h3">Shared keybinds <span class="count">{keys.data?.length ?? ''}</span></h2><button class="btn btn-sm" aria-haspopup="menu" aria-expanded={!!picker} onclick={openPicker}>Add key</button></div>
        <div class="list">
          {#each keys.data ?? [] as k (k.id)}
            <div class="row krow">
              <span class="trunc">{k.name}</span>
              <button class="btn btn-sm" style="justify-content: space-between; font-family: var(--mono)" aria-label="Key for {k.name}" aria-pressed={capturing?.id === k.id} onclick={() => capture(k.id)}><span class="trunc">{capturing?.id === k.id ? 'Press a key…' : k.key}</span></button>
              <button class="btn btn-icon btn-sm btn-ghost" aria-label="Stop sharing {k.name}" onclick={() => edit(k.name, () => api.sync.removeKey(k.id))}><Icon name="x" size={10} sw={2.2} /></button>
            </div>
          {:else}
            {#if keys.data}<Empty text="No shared keybinds yet. Add the keys you want the same everywhere." />{/if}
          {/each}
        </div>
        <p class="small faint" style="margin: 0">Only these keys are written into options.txt on launch. Keys a version doesn't have are skipped.</p>
      </section>
      <div class="page-split">
        <div>
          {#each sections as sec (sec.key)}
            <section class="sec" aria-label={sec.title}>
              <div class="sec-h"><h2 class="h3">{sec.title}</h2><span class="small faint">{sec.note}</span></div>
              <div class="card tgl-grid">
                {#each g[sec.key] as e (e.id)}
                  <ToggleRow name={e.name} hint={e.path} count={users(e, g.total)} on={e.enabled && e.available} disabled={!e.available} title={e.available ? undefined : 'Not installed in any instance'} onchange={(v) => toggle(e, v)} />
                {:else}
                  <Empty text="Nothing to share yet." />
                {/each}
              </div>
            </section>
          {/each}
        </div>
        <div>
          <section class="sec" aria-label="Custom">
            <div class="sec-h"><h2 class="h3">Custom</h2><span class="small faint">Paths inside the instance folder</span></div>
            <div class="card">
              <div class="tgl-grid">
                {#each g.custom as e (e.id)}
                  <ToggleRow name="" hint={e.path} pathOnly count={users(e, g.total)} on={e.enabled} onchange={(v) => toggle(e, v)}>
                    {#snippet after()}<button class="btn btn-icon btn-sm btn-ghost" aria-label="Remove {e.path}" onclick={() => edit(e.path, () => api.sync.removeCustom(e.id))}><Icon name="x" size={10} sw={2.2} /></button>{/snippet}
                  </ToggleRow>
                {/each}
              </div>
              <div class="divider"></div>
              <div style="display: flex; gap: 8px; align-items: center; padding: 12px var(--inset)">
                <label class="input" style="flex: 1 1 auto"><span class="sr">Path inside the instance folder</span><input class="mono" placeholder="config/journeymap/" bind:value={custom} maxlength="200" onkeydown={(e) => e.key === 'Enter' && addCustom(custom.trim().endsWith('/'))} /></label>
                <button class="btn" onclick={() => addCustom(false)}>Add file</button>
                <button class="btn" onclick={() => addCustom(true)}>Add folder</button>
              </div>
            </div>
          </section>
        </div>
      </div>
    {:else}
      <div class="page-split" aria-hidden="true"><div><div class="sk" style="height: 120px; border-radius: var(--radius-lg)"></div><div class="sk" style="height: 160px; border-radius: var(--radius-lg)"></div></div><div><div class="sk" style="height: 140px; border-radius: var(--radius-lg)"></div></div></div>
    {/if}
  </div>
</div>

{#if picker}
  <Menu x={picker.x} y={picker.y} label="Add a shared keybind" scroll onclose={() => (picker = null)}>
    {#if !choices}
      {#each [0, 1, 2] as n (n)}<div class="mi" aria-hidden="true"><span></span><span class="sk" style="height: 10px; width: 60%"></span><span></span></div>{/each}
    {:else}
      {#each byCategory as [category, list], i (category)}
        {#if i > 0}<div class="divider" style="margin: 4px 0"></div>{/if}
        <div class="small faint menu-title trunc">{category}</div>
        {#each list as c (c.id)}
          <button class="mi" role="menuitem" onclick={() => addKey(c.id)}><span></span><span class="trunc">{c.name}</span><span class="small faint mono">{c.key}</span></button>
        {/each}
      {:else}
        <div class="small faint menu-title">Play an instance once to list its keys.</div>
      {/each}
    {/if}
  </Menu>
{/if}

<style>
  .krow{grid-template-columns:minmax(0,1fr) 120px 28px;height:44px}
</style>

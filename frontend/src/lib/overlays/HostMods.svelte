<script lang="ts">
  import { api } from '../api';
  import type { HostMod } from '../api/host';
  import { artOf } from '../look';
  import { session } from '../session.svelte';
  import Check from '../ui/Check.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import SearchField from '../ui/SearchField.svelte';
  import Tile from '../ui/Tile.svelte';

  /** Manage: which mods the server runs (DlgHostMods.dc.html). Rows follow the instance
      Mods table (tile, name, badge, then the select box at the end), shaded every other
      row. The pick is saved for the instance's servers. */
  interface Props { instanceId: string; mods: HostMod[]; onclose: () => void; onsaved: () => void }
  let { instanceId, mods, onclose, onsaved }: Props = $props();

  // svelte-ignore state_referenced_locally
  let on = $state<Record<string, boolean>>(Object.fromEntries(mods.map((m) => [m.key, m.on])));
  let search = $state('');
  let busy = $state(false);
  const shown = $derived(mods.filter((m) => m.name.toLowerCase().includes(search.trim().toLowerCase())));
  const count = $derived(mods.filter((m) => on[m.key]).length);
  const all = (v: boolean) => shown.forEach((m) => (on[m.key] = v));

  async function save() {
    busy = true;
    try {
      await api.host.saveMods(instanceId, { ...on });
      onsaved();
      onclose();
    } catch (e) {
      session.fail('Server mods', e);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog title="Server mods" sub="Checked mods go in the server's mods folder. Your pick is saved for this instance." width={540} {onclose}>
  <div style="display: flex; gap: 6px; align-items: center; flex-wrap: wrap">
    <SearchField bind:value={search} label="Search mods" placeholder="Search {mods.length} mods" style="flex: 1 1 200px" />
    <button class="btn btn-sm btn-ghost" onclick={() => all(true)}>Select all</button>
    <button class="btn btn-sm btn-ghost" onclick={() => all(false)}>Deselect all</button>
  </div>
  <div class="list zebra ml">
    {#each shown as m (m.key)}
      <button class="mr" class:off={!on[m.key]} onclick={() => (on[m.key] = !on[m.key])} aria-pressed={!!on[m.key]}>
        <Tile art={artOf(m.name, m.icon ? `/tex/content/${m.icon}.png` : undefined)} style="width: 28px; height: 28px; flex-basis: 28px; font-size: 12px" />
        <span class="trunc nm">{m.name}</span>
        {#if m.clientOnly}<span class="badge badge-warn">Client only</span>{:else}<span></span>{/if}
        <Check on={!!on[m.key]} />
      </button>
    {:else}
      <div class="mr" style="cursor: default"><span></span><span class="faint">{mods.length ? 'No mods match.' : 'This instance has no mods.'}</span></div>
    {/each}
  </div>
  <span class="field-help">Client-only mods start unchecked; a server can crash on them.</span>
  {#snippet foot()}
    <span class="small faint tnum" style="margin-right: auto; align-self: center">{count} of {mods.length} go on the server</span>
    <button class="btn" onclick={onclose}>Cancel</button>
    <button class="btn btn-accent" disabled={busy} onclick={save}>Save</button>
  {/snippet}
</Dialog>

<style>
  .ml{max-height:330px;overflow-y:auto}
  .mr{display:grid;grid-template-columns:28px minmax(0,1fr) auto 16px;gap:12px;align-items:center;height:44px;padding:0 12px;border:0;background:transparent;width:100%;text-align:left;color:var(--text)}
  .mr.off .nm{color:var(--muted)}
</style>

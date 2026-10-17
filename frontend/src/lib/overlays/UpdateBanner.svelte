<script lang="ts">
  import { api } from '../api';
  import ChangelogDialog from '../components/ChangelogDialog.svelte';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';

  /** A verified launcher update is ready to install. holding keeps its place while the state loads. */
  let { holding = false }: { holding?: boolean } = $props();
  const u = $derived(session.update);
  let notes = $state(false);

  async function restart() {
    try {
      await api.updates.restart();
    } catch (e) {
      session.fail('Restart', e);
    }
  }
</script>

{#if holding}
  <div class="upd" aria-hidden="true"><span class="sk" style="width: 16px; height: 16px; border-radius: 50%"></span><span class="sk" style="height: 12px; width: 320px"></span></div>
{:else}
  <div class="upd" role="status">
    <Icon name="shield-check" sw={1.8} color="var(--accent)" />
    <span class="small"><span style="font-weight: 500">Launcher {u?.version ?? ''} is ready.</span> <span class="muted">Signature verified. Restarting takes about 2 seconds.</span></span>
    <span style="margin-left: auto; display: flex; gap: 6px"><button class="btn btn-sm btn-ghost" onclick={() => (notes = true)}>What changed</button><button class="btn btn-sm btn-ghost" onclick={() => (session.updateLater = true)}>Later</button><button class="btn btn-sm btn-accent" onclick={restart}>Restart now</button></span>
  </div>
{/if}
{#if notes && u}
  <ChangelogDialog title="Quartz Client" meta="{u.current} to {u.version}" versions={[{ id: u.version ?? '', date: Date.parse(u.date ?? '') || 0, items: u.notes }]} onclose={() => (notes = false)}>
    {#snippet foot()}<button class="btn" onclick={() => (notes = false)}>Close</button><button class="btn btn-accent" onclick={restart}>Restart now</button>{/snippet}
  </ChangelogDialog>
{/if}

<style>
  .upd{flex:0 0 44px;display:flex;align-items:center;gap:12px;height:44px;padding:0 12px 0 28px;border-bottom:1px solid var(--line);background:color-mix(in srgb,var(--accent) 7%,var(--bg))}
</style>

<script lang="ts">
  import { api } from '../api';
  import { query } from '../query.svelte';
  import { session } from '../session.svelte';
  import Check from '../ui/Check.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Icon from '../ui/Icon.svelte';
  import { go } from '../router.svelte';

  /** Install a project into the target instance, with its dependencies. */
  interface Props { projectId: string }
  let { projectId }: Props = $props();
  const target = query(() => api.instances.get('fo'));
  let optional = $state(true);
  const count = $derived(optional ? 2 : 1);

  function install() {
    session.close();
    session.tasksRunning = true;
    go({ name: 'downloads' });
  }
</script>

<Dialog title="Install Distant Horizons" onclose={() => session.close()}>
  <div class="kv" style="grid-template-columns: 84px minmax(0,1fr)"><span>Instance</span><span>{target.data?.name ?? ''}</span><span>Version</span><span class="mono">2.3.0-b for {target.data?.minecraft ?? ''}, Fabric</span></div>
  <div style="display: flex; flex-direction: column; gap: 6px">
    <span class="field-label">Also installs</span>
    <div class="list">
      <div class="dep"><Check on={true} /><span class="ico" style="width: 28px; height: 28px; flex-basis: 28px; background: #dbc18e">F</span><span style="min-width: 0"><span class="trunc" style="display: block">Fabric API</span><span class="small faint" style="display: block">Required, already installed</span></span><span class="badge badge-accent">Installed</span></div>
      <div class="dep"><Check bind:on={optional} label="Include Indium" onchange={(v) => (optional = v)} /><span class="ico" style="width: 28px; height: 28px; flex-basis: 28px; background: #c7a3e8">I</span><span style="min-width: 0"><span class="trunc" style="display: block">Indium</span><span class="small faint" style="display: block">Optional, improves Sodium compatibility</span></span><span class="mono small muted">160 KB</span></div>
    </div>
  </div>
  <div class="card" style="padding: 10px 12px; display: grid; grid-template-columns: 16px minmax(0,1fr); gap: 10px; border-color: color-mix(in srgb, var(--info) 30%, var(--line))">
    <Icon name="info" size={16} sw={1.8} color="var(--info)" />
    <span class="small muted">Distant Horizons needs about 1 GB more memory. This instance is set to 6 GB, which is enough.</span>
  </div>
  {#snippet foot()}<button class="btn" onclick={() => session.close()}>Cancel</button><button class="btn btn-accent" onclick={install}>Install {count} mods</button>{/snippet}
</Dialog>

<style>
  .dep{display:grid;grid-template-columns:16px 28px minmax(0,1fr) auto;gap:10px;align-items:center;height:48px;padding:0 12px}
</style>

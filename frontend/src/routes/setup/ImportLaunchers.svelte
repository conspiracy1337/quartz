<script lang="ts">
  import { api } from '$lib/api';
  import { query } from '$lib/query.svelte';
  import { href } from '$lib/router.svelte';
  import Tile from '$lib/ui/Tile.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';
  import SetupLayout from './SetupLayout.svelte';

  const launchers = query(() => api.launchers());
  let picked = $state<Record<string, boolean>>({ 'Prism Launcher': true, 'Modrinth App': true });
  const count = $derived((launchers.data ?? []).filter((l) => picked[l.name]).reduce((n, l) => n + l.instances, 0));
</script>

<SetupLayout step={4} width={500}>
  <div style="display: flex; flex-direction: column; gap: 8px">
    <h1 class="h1" style="font-size: 26px; line-height: 32px">Bring your instances</h1>
    <p class="muted" style="margin: 0">We found other launchers on this computer. Pick what to bring over. The originals are not changed.</p>
  </div>
  <div class="list">
    {#each launchers.data ?? [] as l (l.name)}
      <div class="lr">
        <Tile art={l.art} />
        <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{l.name}</span><span class="trunc small faint" style="display: block">{l.summary}</span></span>
        <Toggle on={!!picked[l.name]} label="Import from {l.name}" onchange={(v) => (picked[l.name] = v)} />
      </div>
    {/each}
  </div>
  <div class="card" style="padding: 12px 14px; display: grid; grid-template-columns: minmax(0,1fr) auto; gap: 12px; align-items: center"><span class="small muted">Java 21 will download in the background while you import.</span><span class="mono small faint">186 MB</span></div>
  <div style="display: flex; justify-content: space-between; align-items: center"><a href={href({ name: 'setup', step: 2 })} class="btn btn-ghost">Back</a><span style="display: flex; gap: 8px"><a href={href({ name: 'home' })} class="btn btn-ghost">Skip</a><a href={href({ name: 'home' })} class="btn btn-accent btn-lg">Import {count} and finish</a></span></div>
</SetupLayout>

<style>
  .lr{display:grid;grid-template-columns:32px minmax(0,1fr) 30px;gap:12px;align-items:center;height:60px;padding:0 14px}
</style>

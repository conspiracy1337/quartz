<script lang="ts">
  import { api } from '$lib/api';
  import type { JavaNeed } from '$lib/api/types';
  import { bytes } from '$lib/format';
  import { prefs } from '$lib/prefs.svelte';
  import { query } from '$lib/query.svelte';
  import { go, href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Empty from '$lib/ui/Empty.svelte';
  import Seg from '$lib/ui/Seg.svelte';
  import Tile from '$lib/ui/Tile.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';
  import SetupLayout from './SetupLayout.svelte';

  const launchers = query(() => api.launchers.list());
  // Every launcher with something new starts picked; the player turns off what to leave.
  let off = $state<Record<string, boolean>>({});
  const picked = $derived((launchers.data ?? []).filter((l) => l.instances > 0 && !off[l.id]));
  const count = $derived(picked.reduce((n, l) => n + l.instances, 0));
  // Composed: copy the instances, or play them in the other launcher's folders.
  const MODES = [{ value: 'copy', label: 'Copy them' }, { value: 'in-place', label: 'Use them in place' }] as const;
  let mode = $state<'copy' | 'in-place'>('copy');

  let java = $state<JavaNeed | null>(null);
  $effect(() => {
    const ids = picked.map((l) => l.id);
    if (!ids.length) {
      java = null;
      return;
    }
    let live = true;
    void api.launchers.java(ids).then((j) => live && (java = j)).catch(() => live && (java = null));
    return () => (live = false);
  });
  const javaText = $derived(java?.majors.length ? `Java ${java.majors.join(' and ')} will download in the background while you import.` : '');

  let busy = $state(false);
  async function finish(importing: boolean) {
    if (busy) return;
    busy = true;
    try {
      if (importing && picked.length) await api.launchers.import(picked.map((l) => l.id), mode === 'in-place');
      await prefs.update((s) => (s.general.setupDone = true));
      go({ name: 'home' });
    } catch (e) {
      session.fail('Import', e);
    } finally {
      busy = false;
    }
  }
</script>

<SetupLayout step={4} width={500}>
  <div style="display: flex; flex-direction: column; gap: 8px">
    <h1 class="h1" style="font-size: 26px; line-height: 32px">Bring your instances</h1>
    <p class="muted" style="margin: 0">We found other launchers on this computer. Pick what to bring over. {mode === 'copy' ? 'The originals are not changed.' : 'They keep playing in the other launcher’s folders.'}</p>
  </div>
  {#if launchers.data && launchers.data.length === 0}
    <Empty text="No other launchers were found on this computer." />
  {:else}
    <div class="list">
      {#if launchers.data}
        {#each launchers.data as l (l.id)}
          <div class="lr">
            <Tile art={l.art} />
            <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{l.name}</span><span class="trunc small faint" style="display: block">{l.summary}</span></span>
            <Toggle on={l.instances > 0 && !off[l.id]} disabled={l.instances === 0} label="Import from {l.name}" onchange={(v) => (off[l.id] = !v)} />
          </div>
        {/each}
      {:else}
        {#each [58, 44] as w (w)}
          <div class="lr" aria-hidden="true"><span class="sk" style="width: 32px; height: 32px; border-radius: 6px"></span><span><span class="sk" style="display: block; height: 12px; width: {w}%"></span><span class="sk" style="display: block; height: 10px; width: {w - 14}%; margin-top: 6px"></span></span><span></span></div>
        {/each}
      {/if}
    </div>
    <Seg options={MODES} bind:value={mode} label="How to bring them over" fill center />
  {/if}
  {#if javaText}
    <div class="card" style="padding: 12px 14px; display: grid; grid-template-columns: minmax(0,1fr) auto; gap: 12px; align-items: center"><span class="small muted">{javaText}</span><span class="mono small faint">{bytes(java?.bytes ?? 0)}</span></div>
  {/if}
  <div style="display: flex; justify-content: space-between; align-items: center"><a href={href({ name: 'setup', step: 2 })} class="btn btn-ghost">Back</a><span style="display: flex; gap: 8px">{#if count > 0}<button class="btn btn-ghost" onclick={() => finish(false)} disabled={busy}>Skip</button><button class="btn btn-accent btn-lg" onclick={() => finish(true)} disabled={busy}>Import {count} and finish</button>{:else}<button class="btn btn-accent btn-lg" onclick={() => finish(false)} disabled={busy || !launchers.data}>Finish</button>{/if}</span></div>
</SetupLayout>

<style>
  .lr{display:grid;grid-template-columns:32px minmax(0,1fr) 30px;gap:12px;align-items:center;height:60px;padding:0 14px}
</style>

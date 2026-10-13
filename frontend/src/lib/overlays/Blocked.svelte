<script lang="ts">
  import { api } from '../api';
  import { onEvent } from '../bridge';
  import { bytes, errorText, plural } from '../format';
  import { artOf } from '../look';
  import { query } from '../query.svelte';
  import { revs } from '../revs.svelte';
  import { session } from '../session.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Icon from '../ui/Icon.svelte';
  import Tile from '../ui/Tile.svelte';

  /** CurseForge files whose authors block third-party downloads, for one job. */
  interface Props { jobId: string }
  let { jobId }: Props = $props();

  const view = query(() => api.manual.view(jobId));
  const items = $derived(view.data?.items ?? []);
  const waiting = $derived(items.filter((i) => i.state === 'waiting'));
  const ready = $derived(!!view.data && items.every((i) => i.state === 'found' || i.state === 'skipped'));

  // Files dropped while the dialog is open are offered to this job.
  $effect(() => onEvent<{ dropId: string }>('content:dropped', (d) => void drop(d.dropId)));

  async function act(fn: () => Promise<unknown>) {
    try {
      await fn();
      revs.bump('manual');
    } catch (e) {
      session.notify({ tone: 'error', subject: "Couldn't use that file", text: errorText(e) });
    }
  }
  const drop = (dropId: string) => act(() => api.manual.addDropped(jobId, dropId));
  async function skip() {
    const n = waiting.length;
    await act(() => api.manual.skip(jobId));
    session.close();
    if (n) session.notify({ tone: 'info', subject: plural(n, 'file'), text: n === 1 ? 'was skipped' : 'were skipped' });
  }
</script>

<Dialog title="{plural(items.length, 'file')} {items.length === 1 ? 'needs' : 'need'} a manual download" sub="Their authors only allow downloads from the CurseForge website." width={580}>
  <div class="card" style="display: flex; align-items: center; gap: 10px; padding: 10px 12px">{#if view.data?.watching === false}<span class="dot" aria-hidden="true"></span><span class="small">Not watching your Downloads folder. Turn it on in Settings, Downloads.</span>{:else}<span class="dot live pulse" aria-hidden="true"></span><span class="small">Watching <span class="mono">{view.data?.folder ?? '~/Downloads'}</span>. Files are picked up as soon as they finish.</span>{/if}</div>
  <div class="list">
    {#each items as it (it.id)}
      <div class="bf">
        {#if it.state === 'found'}<Icon name="check" size={18} sw={2.4} color="var(--accent)" />{:else}<Tile art={it.art ?? artOf(it.name)} style="width: 28px; height: 28px; flex-basis: 28px" />{/if}
        <span style="min-width: 0"><span class="trunc" style="display: block">{it.name}</span><span class="trunc small faint mono" style="display: block">{it.fileName}, {it.state === 'found' ? 'found' : it.state === 'skipped' ? 'skipped' : bytes(it.size)}</span></span>
        {#if it.state === 'found'}<span class="badge badge-accent">Ready</span>
        {:else if it.state === 'skipped'}<span class="badge">Skipped</span>
        {:else}
          <span style="display: flex; gap: 6px">
            {#if it.twin}<button class="btn btn-sm" disabled={it.state === 'fetching'} onclick={() => act(() => api.manual.useModrinth(jobId, it.id))}>Use Modrinth copy</button>{/if}
            {#if it.state === 'waiting' && it.page}{@const page = it.page}<button class="btn btn-sm" onclick={() => void api.app.openUrl(page)}>Open page</button>{/if}
          </span>
        {/if}
      </div>
    {/each}
  </div>
  <span class="small faint">You can also drop the files onto this window.</span>
  {#snippet foot()}<button class="btn" style="margin-right: auto" onclick={skip}>Skip these mods</button><button class="btn" disabled={!waiting.length} onclick={() => act(() => api.manual.openPages(jobId))}>Open all {waiting.length} pages</button><button class="btn btn-accent" disabled={!ready} onclick={() => session.close()}>Continue</button>{/snippet}
</Dialog>

<style>
  .bf{display:grid;grid-template-columns:28px minmax(0,1fr) auto;gap:12px;align-items:center;min-height:60px;padding:8px 12px}
  @media (prefers-reduced-motion:no-preference){.pulse{animation:pl 1.6s ease-in-out infinite}}
  @keyframes pl{50%{opacity:.35}}
</style>

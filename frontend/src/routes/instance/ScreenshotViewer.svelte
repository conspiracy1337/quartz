<script lang="ts">
  import { api } from '$lib/api';
  import { bytes, shortMonthDay, clock } from '$lib/format';
  import { query, skeletonRows } from '$lib/query.svelte';
  import { revs } from '$lib/revs.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';

  interface Props { instanceId: string; shotId: string }
  let { instanceId, shotId }: Props = $props();

  // The grid's key, so a screenshot opened from it shows at once.
  // svelte-ignore state_referenced_locally
  const shots = query(() => api.screenshots.list(instanceId), `screenshots.${instanceId}`);
  let i = $state(0);
  $effect(() => {
    i = Math.max(0, shots.data?.findIndex((s) => s.id === shotId) ?? 0);
  });
  const list = $derived(shots.data ?? []);
  const n = $derived(list.length);
  const cur = $derived(list[i]);
  const when = (t: number) => `${shortMonthDay(t)}, ${clock(t)}`;

  // The viewer shows the full picture, letterboxed; the filmstrip keeps the thumbnails.
  const full = $derived(cur?.image ? `url(${cur.image}) center/contain no-repeat, var(--console)` : cur?.art.background);

  async function run(what: string, fn: () => Promise<unknown>, done?: string) {
    try {
      await fn();
      if (done && cur) session.notify({ subject: cur.file, text: done });
    } catch (e) {
      session.fail(what, e);
    }
  }
  async function trash() {
    if (!cur) return;
    const name = cur.id;
    try {
      const token = await api.screenshots.trash(instanceId, [name]);
      revs.bump('screenshots');
      session.notify({ tone: 'trash', subject: name, text: 'to the trash',
        undo: () => void api.screenshots.undo(instanceId, token).then(() => revs.bump('screenshots')).catch((e: unknown) => session.fail('Undo', e)) });
    } catch (e) {
      session.fail('Move to trash', e);
    }
  }

  function keys(e: KeyboardEvent) {
    // The near-fullscreen view has its own arrows.
    if (!n || session.overlay) return;
    if (e.key === 'ArrowLeft') i = (i + n - 1) % n;
    if (e.key === 'ArrowRight') i = (i + 1) % n;
  }
</script>

<svelte:window onkeydown={keys} />

<div class="vw">
  <section aria-label="Viewer" style="display: flex; flex-direction: column; min-height: 0; min-width: 0">
    <div style="flex: 1 1 auto; min-height: 0; display: grid; grid-template-columns: 40px minmax(0,1fr) 40px; align-items: center; gap: 8px; padding: 16px">
      <button class="btn btn-icon" onclick={() => (i = (i + n - 1) % n)} aria-label="Previous"><Icon name="chevron-left" size={14} sw={2} /></button>
      <div style="height: 100%; display: grid; place-items: center; min-width: 0">{#if cur}{@const shot = cur}<button class="pic" aria-label="Open screenshot {when(shot.takenAt)} larger" style="background: {full}" onclick={() => session.open({ kind: 'shot', instanceId, shotId: shot.id })}></button>{:else}<div class="sk" style="width: 100%; aspect-ratio: 16/9; border-radius: 8px"></div>{/if}</div>
      <button class="btn btn-icon" onclick={() => (i = (i + 1) % n)} aria-label="Next"><Icon name="chevron-right" size={14} sw={2} /></button>
    </div>
    <div class="film" role="listbox" aria-label="Screenshots">
      {#each list as s, k (s.id)}<button class="fi" class:on={k === i} style="background: {s.art.background}" onclick={() => (i = k)} aria-label={when(s.takenAt)} role="option" aria-selected={k === i}></button>
      {:else}{#if !shots.data}{#each { length: skeletonRows(`screenshots.${instanceId}`, 8) } as _, k (k)}<div class="fi sk" aria-hidden="true"></div>{/each}{/if}{/each}
    </div>
  </section>
  <aside aria-label="Details" style="border-left: 1px solid var(--line); padding: 16px 18px; display: flex; flex-direction: column; gap: 14px">
    {#if cur}
      <div><h2 class="h3 mono trunc">{cur.file}</h2><div class="small faint tnum">{when(cur.takenAt)}</div></div>
      <div class="kv small" style="grid-template-columns: 76px minmax(0,1fr); row-gap: 8px"><span>Size</span><span class="mono">{cur.width} × {cur.height}</span><span>File</span><span class="mono">{bytes(cur.bytes)}</span><span>{cur.server ? 'Server' : 'World'}</span><span class="trunc">{cur.server || cur.world}</span></div>
    {:else if !shots.data}
      <div aria-hidden="true"><div class="h3"><span class="sk sk-text" style="height: 12px; width: 160px"></span></div><div class="small"><span class="sk sk-text" style="width: 110px"></span></div></div>
      <div class="kv small" style="grid-template-columns: 76px minmax(0,1fr); row-gap: 8px" aria-hidden="true"><span>Size</span><span><span class="sk sk-text" style="width: 80px"></span></span><span>File</span><span><span class="sk sk-text" style="width: 56px"></span></span><span>World</span><span><span class="sk sk-text" style="width: 96px"></span></span></div>
    {/if}
    <div style="display: flex; flex-direction: column; gap: 6px">
      <button class="btn btn-block" disabled={!cur} onclick={() => cur && run('Show in folder', () => api.screenshots.showInFolder(instanceId, cur.id))}>Show in folder</button>
      <button class="btn btn-block" disabled={!cur} onclick={() => cur && run('Copy image', () => api.screenshots.copy(instanceId, cur.id), 'copied')}>Copy image</button>
      <button class="btn btn-block" disabled={!cur} onclick={() => cur && run('Use as picture', () => api.screenshots.useAsPicture(instanceId, cur.id).then(() => revs.bump('instances')), 'is the instance picture now')}>Use as picture</button>
      <button class="btn btn-block" disabled={!cur} onclick={() => cur && run('Use as banner', () => api.screenshots.useAsBanner(instanceId, cur.id).then(() => revs.bump('instances')), 'is the instance banner now')}>Use as banner</button>
      <button class="btn btn-block btn-danger" disabled={!cur} onclick={trash}>Move to trash</button>
    </div>
  </aside>
</div>

<style>
  .vw{display:grid;grid-template-columns:minmax(0,1fr) 260px;flex:1 1 auto;min-height:0}
  @media (max-width:1100px){.vw{grid-template-columns:minmax(0,1fr)}.vw aside{display:none}}
  .film{display:flex;gap:8px;overflow-x:auto;padding:12px 16px;border-top:1px solid var(--line);flex:0 0 auto}
  .fi{flex:0 0 96px;height:54px;border-radius:6px;border:2px solid transparent;padding:0}
  .fi.on{border-color:var(--text)}
  /* The picture opens near-fullscreen on a click. */
  .pic{width:100%;max-height:100%;aspect-ratio:16/9;border-radius:8px;border:0;padding:0;cursor:zoom-in}
</style>

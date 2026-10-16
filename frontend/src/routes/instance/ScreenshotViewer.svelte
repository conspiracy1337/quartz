<script lang="ts">
  import { api } from '$lib/api';
  import { bytes, shortMonthDay, clock } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import { revs } from '$lib/revs.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';

  interface Props { instanceId: string; shotId: string }
  let { instanceId, shotId }: Props = $props();

  const shots = query(() => api.screenshots.list(instanceId));
  let i = $state(0);
  $effect(() => {
    i = Math.max(0, shots.data?.findIndex((s) => s.id === shotId) ?? 0);
  });
  const list = $derived(shots.data ?? []);
  const n = $derived(list.length);
  const cur = $derived(list[i]);
  const when = (t: number) => `${shortMonthDay(t)}, ${clock(t)}`;

  // The viewer shows the full picture, letterboxed; the filmstrip keeps the thumbnails.
  const full = $derived(cur?.image ? `url(${cur.image}) center/contain no-repeat, #08080a` : cur?.art.background);

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
    if (!n) return;
    if (e.key === 'ArrowLeft') i = (i + n - 1) % n;
    if (e.key === 'ArrowRight') i = (i + 1) % n;
  }
</script>

<svelte:window onkeydown={keys} />

<div class="vw">
  <section aria-label="Viewer" style="display: flex; flex-direction: column; min-height: 0; min-width: 0">
    <div style="flex: 1 1 auto; min-height: 0; display: grid; grid-template-columns: 40px minmax(0,1fr) 40px; align-items: center; gap: 8px; padding: 16px">
      <button class="btn btn-icon" onclick={() => (i = (i + n - 1) % n)} aria-label="Previous"><Icon name="chevron-left" size={14} sw={2} /></button>
      <div style="height: 100%; display: grid; place-items: center; min-width: 0">{#if cur}<div role="img" aria-label="Screenshot {when(cur.takenAt)}" style="width: 100%; max-height: 100%; aspect-ratio: 16/9; border-radius: 8px; background: {full}"></div>{:else}<div class="sk" style="width: 100%; aspect-ratio: 16/9; border-radius: 8px"></div>{/if}</div>
      <button class="btn btn-icon" onclick={() => (i = (i + 1) % n)} aria-label="Next"><Icon name="chevron-right" size={14} sw={2} /></button>
    </div>
    <div class="film" role="listbox" aria-label="Screenshots">
      {#each list as s, k (s.id)}<button class="fi" class:on={k === i} style="background: {s.art.background}" onclick={() => (i = k)} aria-label={when(s.takenAt)} role="option" aria-selected={k === i}></button>{/each}
    </div>
  </section>
  <aside aria-label="Details" style="border-left: 1px solid var(--line); padding: 16px 18px; display: flex; flex-direction: column; gap: 14px">
    {#if cur}
      <div><h2 class="h3 mono trunc">{cur.file}</h2><div class="small faint tnum">{when(cur.takenAt)}</div></div>
      <div class="kv small" style="grid-template-columns: 76px minmax(0,1fr); row-gap: 8px"><span>Size</span><span class="mono">{cur.width} × {cur.height}</span><span>File</span><span class="mono">{bytes(cur.bytes)}</span><span>World</span><span>{cur.world}</span></div>
    {/if}
    <div style="display: flex; flex-direction: column; gap: 6px">
      <button class="btn btn-block" disabled={!cur} onclick={() => cur && run('Copy image', () => api.screenshots.copy(instanceId, cur.id), 'copied')}>Copy image</button>
      <button class="btn btn-block" disabled={!cur} onclick={() => cur && run('Show in folder', () => api.screenshots.showInFolder(instanceId, cur.id))}>Show in folder</button>
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
</style>

<script lang="ts">
  import { api } from '$lib/api';
  import { bytes, shortMonthDay, clock } from '$lib/format';
  import { query, skeletonRows } from '$lib/query.svelte';
  import ZoomPan from '$lib/components/ZoomPan.svelte';
  import { revs } from '$lib/revs.svelte';
  import { go } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';

  interface Props { instanceId: string; shotId: string }
  let { instanceId, shotId }: Props = $props();

  // The grid's key, so a screenshot opened from it shows at once.
  // svelte-ignore state_referenced_locally
  const shots = query(() => api.screenshots.list(instanceId), `screenshots.${instanceId}`);
  const list = $derived(shots.data ?? []);
  const n = $derived(list.length);
  const i = $derived(Math.max(0, list.findIndex((s) => s.id === shotId)));
  const cur = $derived(list[i]);
  const when = (t: number) => `${shortMonthDay(t)}, ${clock(t)}`;
  // The shown one is the route's, so closing fullscreen can leave this page where it stopped.
  const show = (k: number) => n && go({ name: 'instance', id: instanceId, tab: 'screenshots', sub: list[(k + n) % n]!.id }, { replace: true });
  const fullscreen = () => cur && session.open({ kind: 'shot', instanceId, shotId: cur.id });

  // The filmstrip follows the arrows.
  let film = $state<HTMLElement>();
  $effect(() => film?.children[i]?.scrollIntoView({ block: 'nearest', inline: 'nearest' }));
  let zp = $state<ZoomPan>();

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
    // Fullscreen has its own keys, and a focused field keeps its own.
    if (!n || session.overlay || (e.target as Element).closest('input, textarea, select, [contenteditable]')) return;
    if (e.key === 'ArrowLeft') show(i - 1);
    else if (e.key === 'ArrowRight') show(i + 1);
    else if (!zp?.key(e)) return;
    e.preventDefault();
  }
</script>

<svelte:window onkeydown={keys} />

<div class="vw">
  <section aria-label="Viewer" class="vw-main">
    <div class="stage">
      {#if cur}
        <ZoomPan bind:this={zp} src={cur.image} art={cur.art.background} size={{ w: cur.width, h: cur.height }} label="Screenshot {when(cur.takenAt)}. Click for fullscreen." onclick={fullscreen} step={(by) => show(i + by)}>
          {#snippet tools()}<button class="btn btn-sm btn-icon btn-ghost" onclick={fullscreen} aria-label="Fullscreen" title="Fullscreen"><Icon name="expand" size={14} sw={2} /></button>{/snippet}
        </ZoomPan>
      {:else}<div class="sk" style="width: 100%; height: 100%"></div>{/if}
    </div>
    <div class="film" role="listbox" aria-label="Screenshots" bind:this={film}>
      {#each list as s, k (s.id)}<button class="fi" class:on={k === i} style="background: {s.art.background}" onclick={() => show(k)} aria-label={when(s.takenAt)} role="option" aria-selected={k === i}></button>
      {:else}{#if !shots.data}{#each { length: skeletonRows(`screenshots.${instanceId}`, 8) } as _, k (k)}<div class="fi sk" aria-hidden="true"></div>{/each}{/if}{/each}
    </div>
  </section>
  <aside aria-label="Details">
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
  .vw-main{display:flex;flex-direction:column;min-height:0;min-width:0}
  /* The picture takes the page; it letterboxes on the console colour. */
  .stage{flex:1 1 auto;min-height:0;padding:12px}
  .stage > :global(*){border-radius:8px;background:var(--console)}
  .film{display:flex;gap:8px;overflow-x:auto;padding:8px 12px;border-top:1px solid var(--line);flex:0 0 auto}
  .fi{flex:0 0 80px;height:45px;border-radius:6px;border:2px solid transparent;padding:0}
  .fi.on{border-color:var(--text)}
  /* A panel beside the picture, from the tabs down to the bottom. */
  aside{border-left:1px solid var(--line);background:var(--panel);padding:12px 16px;display:flex;flex-direction:column;gap:14px;min-height:0;overflow-y:auto}
</style>

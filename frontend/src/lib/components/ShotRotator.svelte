<script lang="ts">
  import { api } from '../api';
  import { day } from '../format';
  import { query } from '../query.svelte';
  import { href } from '../router.svelte';
  import Icon from '../ui/Icon.svelte';
  import Empty from '../ui/Empty.svelte';

  /** Recent screenshots from every instance, shown one at a time. */

  const INTERVAL = 5000;
  const shots = query(() => api.home.recentShots());
  const instances = query(() => api.instances.list());
  let i = $state(0);
  let paused = $state(false);

  const list = $derived(shots.data ?? []);
  const n = $derived(list.length);
  const cur = $derived(list[i]);
  const where = $derived(cur ? `${instances.data?.find((x) => x.id === cur.instanceId)?.name ?? ''}, ${day(cur.takenAt)}` : '');

  $effect(() => {
    const t = setInterval(() => {
      // Hidden windows (minimised, launcher hidden while playing) don't need to animate.
      if (!paused && !document.hidden && n > 0) i = (i + 1) % n;
    }, INTERVAL);
    return () => clearInterval(t);
  });
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="shr" role="region" aria-roledescription="carousel" aria-label="Recent screenshots" style="font-family: var(--font)"
  onmouseenter={() => (paused = true)} onmouseleave={() => (paused = false)}>
  {#each list as s, k (s.id)}<span class="shr-img" class:on={k === i} style="background: {s.art.background}" aria-hidden="true"></span>{/each}
  {#if cur}
    <a href={href({ name: 'instance', id: cur.instanceId, tab: 'screenshots', sub: cur.id })} class="shr-hit" aria-label="Open {cur.title}"></a>
    <div class="shr-bar" style="padding: 40px 12px 12px 12px">
      <span class="shr-cap" aria-live="polite"><span class="shr-t">{cur.title}</span><span class="shr-d">{where}</span></span>
      <span class="shr-dots">{#each list as s, k (s.id)}<button class:on={k === i} onclick={() => (i = k)} aria-label="Show screenshot {k + 1}"></button>{/each}</span>
      <button class="shr-nv" onclick={() => (i = (i + n - 1) % n)} aria-label="Previous screenshot"><Icon name="chevron-left" size={12} sw={2.2} /></button>
      <button class="shr-nv" onclick={() => (i = (i + 1) % n)} aria-label="Next screenshot"><Icon name="chevron-right" size={12} sw={2.2} /></button>
    </div>
  {:else if shots.data}
    <Empty text="Screenshots you take in any instance show up here." style="position: absolute; inset: 0" />
  {:else}
    <div class="sk" style="position: absolute; inset: 0; border-radius: 0" aria-hidden="true"></div>
  {/if}
</div>

<style>
  /* Classes are prefixed shr- so they never collide with monolith.css (.sr and .bar exist there). */
  .shr{position:absolute;inset:0;border-radius:var(--radius-lg);overflow:hidden;border:1px solid var(--line);background:#0c0c0e;container-type:size}
  .shr-img{position:absolute;inset:0;opacity:0}
  .shr-img.on{opacity:1}
  @media (prefers-reduced-motion:no-preference){.shr-img{transition:opacity 900ms ease}}
  .shr-hit{position:absolute;inset:0;z-index:1}
  .shr-hit:focus-visible{outline:2px solid var(--accent);outline-offset:-2px}
  .shr-bar{position:absolute;left:0;right:0;bottom:0;z-index:2;display:flex;align-items:center;gap:8px;background:linear-gradient(0deg,rgba(6,6,8,.8),rgba(6,6,8,0));color:#fff;pointer-events:none}
  .shr-cap{flex:1 1 auto;min-width:0;display:flex;flex-direction:column}
  .shr-t{font-size:13px;line-height:18px;font-weight:500;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .shr-d{font-size:11px;line-height:16px;color:rgba(255,255,255,.72);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .shr-dots{display:flex;pointer-events:auto}
  .shr-dots button{position:relative;width:12px;height:20px;border:0;padding:0;background:none;transition:width 200ms var(--ease)}
  .shr-dots button::before{content:"";position:absolute;left:3px;right:3px;top:7px;height:6px;border-radius:3px;background:rgba(255,255,255,.4);transition:background-color 200ms ease}
  .shr-dots button.on{width:22px}
  .shr-dots button.on::before{background:#fff}
  .shr-nv{pointer-events:auto;width:28px;height:28px;flex:0 0 28px;border-radius:6px;border:0;background:rgba(0,0,0,.4);color:#fff;display:grid;place-items:center;transition:background-color 120ms ease,transform 140ms var(--ease)}
  .shr-nv:hover{background:rgba(0,0,0,.6)}
  .shr-nv:active{transform:scale(.97)}
  @container (max-width:260px){.shr-dots{display:none}}
</style>

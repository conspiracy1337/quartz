<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../api';
  import ZoomPan from '../components/ZoomPan.svelte';
  import { clock, shortMonthDay } from '../format';
  import { enterFullscreen } from '../fullscreen';
  import { query } from '../query.svelte';
  import { go } from '../router.svelte';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';

  /**
   * A screenshot fullscreen on the monitor, over the whole window, zoomable like the viewer.
   * Escape or × closes it, leaving the viewer page on the screenshot shown last; the arrow
   * keys and buttons step.
   * The controls fade while the mouse rests.
   */
  interface Props { instanceId: string; shotId: string }
  let { instanceId, shotId }: Props = $props();

  // The viewer's list, already loaded under this key.
  // svelte-ignore state_referenced_locally
  const shots = query(() => api.screenshots.list(instanceId), `screenshots.${instanceId}`);
  const list = $derived(shots.data ?? []);
  const n = $derived(list.length);
  // svelte-ignore state_referenced_locally
  let id = $state(shotId);
  const i = $derived(Math.max(0, list.findIndex((s) => s.id === id)));
  const cur = $derived(list[i]);
  const step = (by: number) => n && (id = list[(i + by + n) % n]!.id);
  // Navigating closes dialogs, so the page moves to this screenshot only on the way out.
  function close() {
    if (id !== shotId) go({ name: 'instance', id: instanceId, tab: 'screenshots', sub: id }, { replace: true });
    session.close();
  }

  let el: HTMLElement;
  let zp = $state<ZoomPan>();
  // Leaving restores the window however the view closes.
  onMount(() => {
    el.focus();
    let restore: (() => void) | null = null, gone = false;
    void enterFullscreen().then((r) => (gone ? r() : (restore = r)));
    return () => {
      gone = true;
      restore?.();
    };
  });

  let idle = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;
  function wake() {
    idle = false;
    clearTimeout(timer);
    timer = setTimeout(() => (idle = true), 2000);
  }
  onMount(() => (wake(), () => clearTimeout(timer)));

  function keys(e: KeyboardEvent) {
    if (e.key === 'Escape') close();
    else if (e.key === 'ArrowLeft') step(-1);
    else if (e.key === 'ArrowRight') step(1);
    else if (!zp?.key(e)) return;
    e.preventDefault();
    e.stopPropagation();
  }
</script>

<div class="fs" class:idle bind:this={el} tabindex="-1" role="dialog" aria-modal="true" aria-label={cur ? `${cur.title}, ${shortMonthDay(cur.takenAt)}` : 'Screenshot'} onkeydown={keys} onpointermove={wake} onpointerdown={wake}>
  {#if cur}<ZoomPan bind:this={zp} src={cur.image} art={cur.art.background} size={{ w: cur.width, h: cur.height }} label="Screenshot {shortMonthDay(cur.takenAt)}, {clock(cur.takenAt)}" step={n > 1 ? step : undefined} />{/if}
  <div class="fs-head">
    <span class="trunc" style="min-width: 0">{#if cur}<span style="font-weight: 500">{cur.title}</span> <span class="small faint">{shortMonthDay(cur.takenAt)}, {clock(cur.takenAt)}</span>{/if}</span>
    <span class="small faint tnum">{n ? `${i + 1} of ${n}` : ''}</span>
    <button class="btn btn-icon btn-sm btn-ghost" aria-label="Close" title="Close (Esc)" onclick={close}><Icon name="x" size={12} sw={2} /></button>
  </div>
</div>

<style>
  .fs{position:fixed;inset:0;z-index:50;background:var(--console);display:flex}
  .fs:focus{outline:none}
  /* The zoom bar's look, top right. */
  .fs-head{position:absolute;top:12px;right:12px;display:flex;align-items:center;gap:12px;max-width:min(560px,calc(100% - 24px));padding:4px 4px 4px 12px;border-radius:8px;background:color-mix(in srgb,var(--panel) 80%,transparent);border:1px solid var(--line)}
  .fs :global(.zp-edge),.fs :global(.zp-bar),.fs-head{transition:opacity 200ms ease}
  .fs.idle :global(.zp-edge),.fs.idle :global(.zp-bar),.fs.idle .fs-head{opacity:0}
  .fs.idle,.fs.idle :global(.zp){cursor:none}
  @media (prefers-reduced-motion:no-preference){.fs{animation:fs-in 180ms cubic-bezier(.23,1,.32,1)}}
  @keyframes fs-in{from{opacity:0}to{opacity:1}}
</style>

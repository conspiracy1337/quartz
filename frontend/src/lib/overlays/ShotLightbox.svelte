<script lang="ts">
  import { api } from '../api';
  import { clock, shortMonthDay } from '../format';
  import { query } from '../query.svelte';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';

  /**
   * A screenshot near-fullscreen (composed: the dialog scrim, the full picture letterboxed,
   * the viewer's arrows). Escape, × or a click beside the picture closes it; the arrow
   * keys and buttons step through the instance's screenshots.
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
  let el: HTMLElement;
  $effect(() => el.focus());

  function keys(e: KeyboardEvent) {
    if (e.key === 'Escape') session.close();
    else if (e.key === 'ArrowLeft') step(-1);
    else if (e.key === 'ArrowRight') step(1);
    else return;
    e.preventDefault();
    e.stopPropagation();
  }
</script>

<!-- A click on the scrim, beside the picture, closes it (Escape and × do too). -->
<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="scrim lb" onclick={(e) => e.target === e.currentTarget && session.close()}>
  <div class="lb-box" bind:this={el} tabindex="-1" role="dialog" aria-modal="true" aria-label={cur ? `${cur.title}, ${shortMonthDay(cur.takenAt)}` : 'Screenshot'} onkeydown={keys}>
    <div class="lb-head">
      <span class="trunc" style="min-width: 0">{#if cur}<span style="font-weight: 500">{cur.title}</span> <span class="small faint">{shortMonthDay(cur.takenAt)}, {clock(cur.takenAt)}</span>{/if}</span>
      <span class="small faint tnum">{n ? `${i + 1} of ${n}` : ''}</span>
      <button class="btn btn-icon btn-sm btn-ghost" aria-label="Close" onclick={() => session.close()}><Icon name="x" size={12} sw={2} /></button>
    </div>
    <div class="lb-stage">
      <button class="btn btn-icon" aria-label="Previous" disabled={n < 2} onclick={() => step(-1)}><Icon name="chevron-left" size={14} sw={2} /></button>
      {#if cur?.image}<img class="lb-img" src={cur.image} alt="" draggable="false" />
      {:else}<div class="lb-img" style="background: {cur?.art.background ?? 'var(--console)'}; aspect-ratio: 16/9; width: 100%"></div>{/if}
      <button class="btn btn-icon" aria-label="Next" disabled={n < 2} onclick={() => step(1)}><Icon name="chevron-right" size={14} sw={2} /></button>
    </div>
  </div>
</div>

<style>
  .lb{padding:16px;z-index:40}
  .lb-box{width:100%;height:100%;display:flex;flex-direction:column;border:1px solid var(--line-2);border-radius:12px;background:var(--console);box-shadow:0 24px 64px -16px rgba(0,0,0,.7);overflow:hidden}
  .lb-box:focus{outline:none}
  .lb-head{display:grid;grid-template-columns:minmax(0,1fr) auto 28px;align-items:center;gap:12px;height:44px;padding:0 8px 0 16px;border-bottom:1px solid var(--line);background:var(--panel)}
  .lb-stage{flex:1 1 auto;min-height:0;display:grid;grid-template-columns:32px minmax(0,1fr) 32px;align-items:center;gap:12px;padding:12px}
  .lb-img{max-width:100%;max-height:100%;width:auto;height:auto;justify-self:center;object-fit:contain;border-radius:6px;user-select:none}
  @media (prefers-reduced-motion:no-preference){.lb-box{animation:lb-in 180ms cubic-bezier(.23,1,.32,1)}}
  @keyframes lb-in{from{opacity:0;transform:scale(.985)}to{opacity:1;transform:none}}
</style>

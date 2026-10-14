<script lang="ts">
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';

  import type { Toast } from '../session.svelte';

  /** "Moved X to the trash" with Undo, or a short notice with an optional action;
      the bar drains while it waits. */
  let { subject, text, undo, tone = 'info', action }: Toast = $props();
  const icon = $derived(tone === 'trash' ? 'trash' : tone === 'error' ? 'warn' : 'info');
</script>

<!-- Hover or focus holds it; the bar pauses with it. -->
<div class="toast tst" role={tone === 'error' ? 'alert' : 'status'}
  onpointerenter={() => session.holdToast(true)} onpointerleave={() => session.holdToast(false)}
  onfocusin={() => session.holdToast(true)} onfocusout={() => session.holdToast(false)}>
  <Icon name={icon} style="color: {tone === 'error' ? 'var(--warn)' : 'var(--muted)'}" />
  {#if tone === 'trash'}
    <span>Moved <span style="font-weight: 500">{subject}</span> {text}</span>
  {:else}
    <span><span style="font-weight: 500">{subject}</span> <span class="muted">{text}</span></span>
  {/if}
  {#if undo}<button class="btn btn-sm" style="margin-left: 8px" onclick={() => { undo?.(); session.dismissToast(); }}>Undo</button>{/if}
  {#if action}<button class="btn btn-sm" style="margin-left: 8px" onclick={() => { action?.run(); session.dismissToast(); }}>{action.label}</button>{/if}
  <button class="btn btn-icon btn-sm btn-ghost" aria-label="Dismiss" onclick={() => session.dismissToast()}><Icon name="x" size={10} sw={2.2} /></button>
  <span class="tbar" aria-hidden="true"><span></span></span>
</div>

<style>
  .tst{position:absolute;left:50%;bottom:24px;transform:translateX(-50%);z-index:10}
  @media (prefers-reduced-motion:no-preference){.tst{animation:tin 260ms cubic-bezier(.23,1,.32,1)}.tbar > span{animation:drain 8s linear forwards}}
  @keyframes tin{from{opacity:0;transform:translate(-50%,10px)}to{opacity:1;transform:translateX(-50%)}}
  @keyframes drain{from{width:100%}to{width:0%}}
  .tbar{position:absolute;left:8px;right:8px;bottom:0;height:2px;border-radius:1px;overflow:hidden}
  .tbar > span{display:block;height:100%;background:var(--muted);width:100%}
  .tst:hover .tbar > span,.tst:focus-within .tbar > span{animation-play-state:paused}
</style>

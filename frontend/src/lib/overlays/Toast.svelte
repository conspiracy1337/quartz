<script lang="ts">
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';

  /** "Moved X to the trash" with Undo; the bar drains while it waits. */
  interface Props { subject: string; text: string; undo?: (() => void) | undefined }
  let { subject, text, undo }: Props = $props();
</script>

<div class="toast tst" role="status">
  <Icon name="trash" style="color: var(--muted)" />
  <span>Moved <span style="font-weight: 500">{subject}</span> {text}</span>
  {#if undo}<button class="btn btn-sm" style="margin-left: 8px" onclick={() => { undo?.(); session.dismissToast(); }}>Undo</button>{/if}
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
</style>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  interface Props {
    title: string;
    sub?: string;
    width?: number;
    alert?: boolean;
    /** Shows the close button and closes on Escape. */
    onclose?: () => void;
    head?: Snippet;
    foot?: Snippet;
    children: Snippet;
  }
  let { title, sub, width = 520, alert = false, onclose, head, foot, children }: Props = $props();
  const id = `dlg-${Math.random().toString(36).slice(2, 8)}`;
  let el: HTMLElement;

  $effect(() => el.focus());

  function keys(e: KeyboardEvent) {
    if (e.key === 'Escape' && onclose) {
      e.stopPropagation();
      onclose();
    }
  }
</script>

<div class="scrim">
  <section class="dialog" bind:this={el} tabindex="-1" role={alert ? 'alertdialog' : 'dialog'} aria-modal="true" aria-labelledby={head ? undefined : id} aria-label={head ? title : undefined} style="width: min({width}px, 100%)" onkeydown={keys}>
    <div class="dialog-head">
      {#if head}
        {@render head()}
      {:else if sub}
        <span><span class="h2" {id} style="display: block">{title}</span><span class="small faint">{sub}</span></span>
      {:else}
        <h2 class="h2" {id}>{title}</h2>
      {/if}
      {#if onclose}
        <button class="btn btn-icon btn-sm btn-ghost" aria-label="Close" onclick={onclose}><Icon name="x" size={12} sw={2} /></button>
      {/if}
    </div>
    <div class="dialog-body">{@render children()}</div>
    {#if foot}<div class="dialog-foot">{@render foot()}</div>{/if}
  </section>
</div>

<style>
  .dialog:focus{outline:none}
  @media (prefers-reduced-motion:no-preference){.dialog{animation:pop 200ms cubic-bezier(.23,1,.32,1)}}
  @keyframes pop{from{opacity:0;transform:scale(.97) translateY(4px)}to{opacity:1;transform:none}}
</style>

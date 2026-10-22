<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  /**
   * A button with a ▾ beside it for its other choices: Home's Join (another
   * instance), the project page's Install (another instance) and a content tab's
   * Add (from a file). The main part is a button, or a link when href is set; the
   * ▾ calls onmenu and the caller shows its menu. Without onmenu there is no ▾ (an
   * instance's Join), and the button fills the width the caller's style gives it.
   */
  interface Props {
    size?: 'sm' | 'md' | 'lg';
    /** Both halves are the accent button, so the pair always reads as one control. */
    accent?: boolean;
    href?: string | undefined;
    disabled?: boolean;
    menuLabel?: string;
    expanded?: boolean | undefined;
    onclick?: () => void;
    onmenu?: ((e: MouseEvent) => void) | undefined;
    style?: string | undefined;
    children: Snippet;
  }
  let { size = 'md', accent = false, href, disabled = false, menuLabel, expanded, onclick, onmenu, style, children }: Props = $props();
</script>

<span class="split split-{size}" {style}>
  {#if href}
    <a {href} class="btn" class:btn-accent={accent} class:btn-sm={size === 'sm'} class:btn-lg={size === 'lg'}>{@render children()}</a>
  {:else}
    <button class="btn" class:btn-accent={accent} class:btn-sm={size === 'sm'} class:btn-lg={size === 'lg'} {disabled} {onclick}>{@render children()}</button>
  {/if}
  {#if onmenu}
    <button class="btn" class:btn-accent={accent} class:btn-sm={size === 'sm'} class:btn-lg={size === 'lg'} class:btn-icon={size === 'lg'}
      aria-label={menuLabel} aria-haspopup="menu" aria-expanded={expanded} onclick={onmenu}>
      <Icon name="chevron-down" size={size === 'lg' ? 12 : 10} sw={size === 'lg' ? 2.2 : 2.4} />
    </button>
  {/if}
</span>

<style>
  /* Pressing either half presses the whole button, so the two halves never pull apart. */
  .split{display:inline-flex;transition:transform 140ms var(--ease)}
  .split:has(> .btn:active:not(:disabled)){transform:scale(.97)}
  .split > .btn:active{transform:none}
  .split > :first-child:not(:only-child){border-radius:6px 0 0 6px}
  .split > :last-child:not(:only-child){border-radius:0 6px 6px 0;padding:0}
  .split-sm > :last-child:not(:only-child){width:24px}
  .split-md > :last-child:not(:only-child){width:28px}
  .split-lg > :last-child:not(:only-child){width:36px}
  .split > :last-child:not(:only-child):not(.btn-accent){border-left:0}
  .split > .btn-accent:last-child:not(:only-child){border-left:1px solid rgba(0,0,0,.2)}
  .split > :only-child{flex:1 1 auto}
</style>

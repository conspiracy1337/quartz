<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  /**
   * A button with a ▾ beside it for its other choices: Home's Join (another
   * instance), the project page's Install (another instance) and a content tab's
   * Add (from a file). The main part is a button, or a link when href is set; the
   * ▾ calls onmenu and the caller shows its menu.
   */
  interface Props {
    size?: 'sm' | 'md' | 'lg';
    /** The main part is the accent button; the ▾ follows unless menuAccent says otherwise. */
    accent?: boolean;
    menuAccent?: boolean;
    href?: string | undefined;
    disabled?: boolean;
    menuDisabled?: boolean;
    menuLabel: string;
    expanded?: boolean | undefined;
    onclick?: () => void;
    onmenu: (e: MouseEvent) => void;
    style?: string | undefined;
    children: Snippet;
  }
  let { size = 'md', accent = false, menuAccent = accent, href, disabled = false, menuDisabled = false, menuLabel, expanded, onclick, onmenu, style, children }: Props = $props();
</script>

<span class="split split-{size}" {style}>
  {#if href}
    <a {href} class="btn" class:btn-accent={accent} class:btn-sm={size === 'sm'} class:btn-lg={size === 'lg'}>{@render children()}</a>
  {:else}
    <button class="btn" class:btn-accent={accent} class:btn-sm={size === 'sm'} class:btn-lg={size === 'lg'} {disabled} {onclick}>{@render children()}</button>
  {/if}
  <button class="btn" class:btn-accent={menuAccent} class:btn-sm={size === 'sm'} class:btn-lg={size === 'lg'} class:btn-icon={size === 'lg'}
    aria-label={menuLabel} aria-haspopup="menu" aria-expanded={expanded} disabled={menuDisabled} onclick={onmenu}>
    <Icon name="chevron-down" size={size === 'lg' ? 12 : 10} sw={size === 'lg' ? 2.2 : 2.4} />
  </button>
</span>

<style>
  .split{display:inline-flex}
  .split > :first-child{border-radius:6px 0 0 6px}
  .split > :last-child{border-radius:0 6px 6px 0;padding:0}
  .split-sm > :last-child{width:24px}
  .split-md > :last-child{width:28px}
  .split-lg > :last-child{width:36px}
  .split > :last-child:not(.btn-accent){border-left:0}
  .split > .btn-accent:last-child{border-left:1px solid rgba(0,0,0,.2)}
</style>

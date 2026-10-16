<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  /** One .setting row: title and optional description on the left, a control on the right.
      With href the row opens a sub-page, and a chevron after the control says so. */
  interface Props {
    title: string;
    desc?: string;
    /** Keeps the description on one line (cards that must never grow). */
    oneLine?: boolean;
    href?: string;
    class?: string;
    style?: string;
    children?: Snippet;
  }
  let { title, desc, oneLine = false, href, class: cls = '', style, children }: Props = $props();
</script>

{#snippet text()}
  <span><span class="setting-title" style="display: block">{title}</span>{#if desc}<span class="setting-desc" class:trunc={oneLine} style="display: block">{desc}</span>{/if}</span>
{/snippet}

{#if href}
  <a {href} class="setting {cls}" style="text-decoration: none; {style ?? ''}">{@render text()}<span class="set-go">{@render children?.()}<Icon name="chevron-right" size={12} sw={2} /></span></a>
{:else}
  <div class="setting {cls}" {style}>{@render text()}{@render children?.()}</div>
{/if}

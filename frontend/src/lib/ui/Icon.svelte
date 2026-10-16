<script lang="ts">
  import { icons, type IconName } from './icons';

  interface Props {
    name: IconName;
    size?: number;
    /** Stroke width; ignored by filled icons. */
    sw?: number;
    /** Draws a filled icon as an outline instead (the activity feed's play mark). */
    outline?: boolean;
    /** Stroke (or fill) colour; defaults to the text colour. */
    color?: string;
    class?: string;
    style?: string;
  }
  let { name, size = 16, sw = 1.6, outline = false, color = 'currentColor', class: cls = '', style = '' }: Props = $props();
  const def = $derived(icons[name]);
</script>

{#if 'brand' in def}
  <svg width={size} height={size} viewBox="0 0 24 24" class={cls} {style} aria-hidden="true">{@html def.body}</svg>
{:else if 'fill' in def && !outline}
  <svg width={size} height={size} viewBox="0 0 24 24" fill={color} class={cls} {style} aria-hidden="true">{@html def.body}</svg>
{:else}
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} stroke-width={sw} class={cls} {style} aria-hidden="true">{@html def.body}</svg>
{/if}

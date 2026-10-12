<script lang="ts">
  import { icons, type IconName } from './icons';

  interface Props {
    name: IconName;
    size?: number;
    /** Stroke width; ignored by filled icons. */
    sw?: number;
    /** Stroke (or fill) colour; defaults to the text colour. */
    color?: string;
    class?: string;
    style?: string;
  }
  let { name, size = 16, sw = 1.6, color = 'currentColor', class: cls = '', style = '' }: Props = $props();
  const def = $derived(icons[name]);
</script>

{#if 'fill' in def}
  <svg width={size} height={size} viewBox="0 0 24 24" fill={color} class={cls} {style} aria-hidden="true">{@html def.body}</svg>
{:else}
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} stroke-width={sw} class={cls} {style} aria-hidden="true">{@html def.body}</svg>
{/if}

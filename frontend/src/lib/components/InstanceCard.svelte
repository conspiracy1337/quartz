<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { Art } from '../api/types';
  import { coverOf, isPicture } from '../look';
  import InstanceMark from '../ui/InstanceMark.svelte';

  /** A Library card: the banner (or a glow of the tile colour), the tile, name, meta and a
      footer line. Library adds its link and Play button as children; the look editor
      shows the same card as its preview, so the two never drift apart. */
  interface Props {
    art: Art;
    icon?: string | undefined;
    banner?: string | undefined;
    name: string;
    meta: string;
    left: string;
    right: string;
    oncontextmenu?: (e: MouseEvent) => void;
    children?: Snippet;
  }
  let { art, icon, banner, name, meta, left, right, oncontextmenu, children }: Props = $props();
</script>

<div class="icard" role="group" aria-label={name} {oncontextmenu}>
  <div class="icard-cover" style="background: {coverOf(art, banner)}" style:opacity={isPicture(banner) ? undefined : 0.9}></div>
  <div style="padding: 0 14px 12px; display: flex; flex-direction: column; justify-content: space-between; min-width: 0">
    <span class="ico ico-lg" style="background: {art.background}; margin-top: -24px; box-shadow: 0 0 0 3px var(--panel); position: relative; z-index: 1"><InstanceMark {art} {icon} /></span>
    <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{name}</span><span class="trunc small muted" style="display: block">{meta || '\u00a0'}</span></span>
    <span class="small faint tnum" style="display: flex; justify-content: space-between"><span>{left}</span><span>{right}</span></span>
  </div>
  {@render children?.()}
</div>

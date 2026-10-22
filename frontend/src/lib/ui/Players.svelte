<script lang="ts">
  import HoverCard from './HoverCard.svelte';

  /** A server's player count; when the server lists who is on, hovering it shows them. */
  interface Props { text: string; online: number; sample?: string[] | undefined }
  let { text, online, sample }: Props = $props();
  const more = $derived(online - (sample?.length ?? 0));
</script>

{#if sample?.length}
  <HoverCard label="{text}, players on">
    {text}
    {#snippet card()}
      <div class="small faint menu-title">Players on</div>
      {#each sample as name, i (i)}<div class="pl trunc">{name}</div>{/each}
      {#if more > 0}<div class="pl small faint">and {more.toLocaleString()} more</div>{/if}
    {/snippet}
  </HoverCard>
{:else}
  {text}
{/if}

<style>
  .pl{padding:2px 8px;font-size:12px;line-height:18px;font-family:var(--font);text-align:left;color:var(--text)}
  .pl.faint{color:var(--faint)}
</style>

<script lang="ts">
  /** Plain text where `backticks` become <code>. Renders text nodes only, never HTML. */
  interface Props { text: string }
  let { text }: Props = $props();
  const parts = $derived(text.split('`'));
</script>

{#each parts as part, i (i)}{#if i % 2 === 1}<code>{part}</code>{:else}{part}{/if}{/each}

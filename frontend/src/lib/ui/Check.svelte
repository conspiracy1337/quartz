<script lang="ts">
  import Icon from './Icon.svelte';

  /** A checkbox. Without onchange it renders as a static, non-interactive mark. */
  interface Props { on: boolean; label?: string; onchange?: (on: boolean) => void }
  let { on = $bindable(), label = '', onchange }: Props = $props();
</script>

{#if onchange}
  <button class="check" class:on aria-pressed={on} aria-label={label} onclick={() => onchange((on = !on))}>
    {#if on}<Icon name="check" size={10} sw={3.5} />{/if}
  </button>
{:else}
  <span class="check" class:on aria-hidden="true">{#if on}<Icon name="check" size={10} sw={3.5} />{/if}</span>
{/if}

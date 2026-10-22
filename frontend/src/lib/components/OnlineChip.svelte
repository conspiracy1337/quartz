<script lang="ts">
  import { onlineOnly } from '../servers.svelte';

  /** A servers table's Online filter (key is its scope's): Home has it over Join, an instance's tab beside its search. */
  interface Props { key: string }
  let { key }: Props = $props();
  const on = $derived(!!onlineOnly[key]);
</script>

<button class="chip" class:on aria-pressed={on} onclick={() => (onlineOnly[key] = !on)}>Online</button>

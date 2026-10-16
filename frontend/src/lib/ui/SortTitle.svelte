<script lang="ts">
  import Icon from './Icon.svelte';

  /** A table column's title that sorts by it: dir 1 or -1 while it sorts (arrow shown), 0 when not. */
  interface Props { label: string; dir: 0 | 1 | -1; onclick: () => void }
  let { label, dir, onclick }: Props = $props();
</script>

<button class="th" class:on={dir !== 0} aria-label="Sort by {label}{dir === 1 ? ', sorted' : dir === -1 ? ', sorted in reverse' : ''}" {onclick}>
  <span>{label}</span><Icon name={dir === -1 ? 'chevron-down' : 'chevron-up'} size={12} sw={2} class="th-ar" />
</button>

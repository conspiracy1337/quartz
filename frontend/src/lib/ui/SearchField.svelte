<script lang="ts">
  import Icon from './Icon.svelte';

  interface Props { value?: string; label: string; placeholder?: string; style?: string; height?: number }
  let { value = $bindable(''), label, placeholder = '', style = '', height }: Props = $props();
</script>

<label class="input" style={height ? `${style}; height: ${height}px` : style}>
  <Icon name="search" size={14} sw={1.8} />
  <span class="sr">{label}</span>
  <input type="search" {placeholder} bind:value />
</label>

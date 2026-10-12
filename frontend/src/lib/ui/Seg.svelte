<script lang="ts" generics="T extends string | number">
  /** Segmented control. `fill` stretches it and its buttons to the full width. */
  interface Props {
    options: readonly { value: T; label: string }[];
    value: T;
    label?: string;
    fill?: boolean;
    onchange?: (v: T) => void;
  }
  let { options, value = $bindable(), label, fill = false, onchange }: Props = $props();

  // Assign first: inside `onchange?.(value = v)` the assignment is skipped when no
  // onchange is passed, which silently broke bind:value.
  function pick(v: T) {
    value = v;
    onchange?.(v);
  }
</script>

<div class="seg" role="radiogroup" aria-label={label} style={fill ? 'width: 100%' : undefined}>
  {#each options as o (o.value)}
    <button
      class:on={value === o.value}
      role="radio"
      aria-checked={value === o.value}
      style={fill ? 'flex: 1 1 0; justify-content: center' : undefined}
      onclick={() => pick(o.value)}>{o.label}</button>
  {/each}
</div>

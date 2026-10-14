<script lang="ts" generics="T extends string | number">
  /** Segmented control. `fill` stretches it and its buttons to the full width; `center` centres
   *  their labels (New instance's loaders) instead of starting them at the left (Add account's tabs). */
  interface Props {
    /** A disabled option dims like a disabled button and can't be picked. */
    options: readonly { value: T; label: string; disabled?: boolean }[];
    value: T;
    label?: string;
    fill?: boolean;
    center?: boolean;
    onchange?: (v: T) => void;
  }
  let { options, value = $bindable(), label, fill = false, center = false, onchange }: Props = $props();

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
      disabled={o.disabled}
      style={fill ? `flex: 1 1 0${center ? '; justify-content: center' : ''}` : undefined}
      onclick={() => pick(o.value)}>{o.label}</button>
  {/each}
</div>

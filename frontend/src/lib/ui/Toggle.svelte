<script lang="ts">
  interface Props { on: boolean; label: string; disabled?: boolean; onchange?: (on: boolean) => void }
  let { on = $bindable(), label, disabled = false, onchange }: Props = $props();
  function flip() {
    on = !on;
    onchange?.(on);
  }
</script>

<button class="tg" class:on aria-pressed={on} aria-label={label} {disabled} onclick={flip}><span></span></button>

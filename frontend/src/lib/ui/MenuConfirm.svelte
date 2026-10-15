<script lang="ts">
  /** A destructive action asking once more inside a Menu: what happens, then Cancel (focused first) and the action. */
  interface Props { text: string; action: string; oncancel: () => void; onconfirm: () => void }
  let { text, action, oncancel, onconfirm }: Props = $props();
</script>

<div class="small muted menu-title">{text}</div>
<div style="display: flex; gap: 6px; padding: 4px 8px 6px"><button class="btn btn-sm" role="menuitem" onclick={oncancel}>Cancel</button><button class="btn btn-sm btn-danger" role="menuitem" onclick={onconfirm}>{action}</button></div>

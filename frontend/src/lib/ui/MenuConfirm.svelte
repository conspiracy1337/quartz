<script lang="ts">
  /** An action asking once more inside a Menu: what happens, then Cancel (focused first) and the action,
      red when it destroys something (danger, the default), the accent colour when it only sends something out. */
  interface Props { text: string; action: string; danger?: boolean; oncancel: () => void; onconfirm: () => void }
  let { text, action, danger = true, oncancel, onconfirm }: Props = $props();
</script>

<div class="small muted menu-title">{text}</div>
<div style="display: flex; gap: 6px; padding: 4px 8px 6px"><button class="btn btn-sm" role="menuitem" onclick={oncancel}>Cancel</button><button class="btn btn-sm" class:btn-danger={danger} class:btn-accent={!danger} role="menuitem" onclick={onconfirm}>{action}</button></div>

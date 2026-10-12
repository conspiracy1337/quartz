<script lang="ts">
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';

  /** A verified launcher update is ready to install. */
</script>

<div class="upd" role="status">
  <Icon name="shield-check" sw={1.8} color="var(--accent)" />
  <span class="small"><span style="font-weight: 500">Launcher 1.4.0 is ready.</span> <span class="muted">Signature verified. Restarting takes about 2 seconds.</span></span>
  <span style="margin-left: auto; display: flex; gap: 6px"><button class="btn btn-sm btn-ghost">What changed</button><button class="btn btn-sm btn-ghost" onclick={() => (session.updateReady = false)}>Later</button><button class="btn btn-sm btn-accent">Restart now</button></span>
</div>

<style>
  .upd{flex:0 0 44px;display:flex;align-items:center;gap:12px;height:44px;padding:0 12px 0 28px;border-bottom:1px solid var(--line);background:color-mix(in srgb,var(--accent) 7%,var(--bg))}
</style>

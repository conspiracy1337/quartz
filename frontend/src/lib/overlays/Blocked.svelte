<script lang="ts">
  import { api } from '../api';
  import { session } from '../session.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Icon from '../ui/Icon.svelte';

  /** CurseForge files whose authors block third-party downloads. */
  const open = (url: string) => void api.app.openUrl(url);
  const PAGES = ['https://www.curseforge.com/minecraft/mc-mods/balm', 'https://www.curseforge.com/minecraft/mc-mods/controlling'];
</script>

<Dialog title="3 files need a manual download" sub="Their authors only allow downloads from the CurseForge website." width={580}>
  <div class="card" style="display: flex; align-items: center; gap: 10px; padding: 10px 12px"><span class="dot live pulse" aria-hidden="true"></span><span class="small">Watching <span class="mono">~/Downloads</span>. Files are picked up as soon as they finish.</span></div>
  <div class="list">
    <div class="bf"><Icon name="check" size={18} sw={2.4} color="var(--accent)" /><span style="min-width: 0"><span class="trunc" style="display: block">Waystones</span><span class="trunc small faint mono" style="display: block">waystones-forge-1.16.5-7.6.4.jar, found</span></span><span class="badge badge-accent">Ready</span></div>
    <div class="bf"><span class="ico" style="width: 28px; height: 28px; flex-basis: 28px; background: #e2a03f">B</span><span style="min-width: 0"><span class="trunc" style="display: block">Balm</span><span class="trunc small faint mono" style="display: block">balm-1.16.5-1.6.2.jar, 312 KB</span></span><span style="display: flex; gap: 6px"><button class="btn btn-sm">Use Modrinth copy</button><button class="btn btn-sm" onclick={() => open(PAGES[0]!)}>Open page</button></span></div>
    <div class="bf"><span class="ico" style="width: 28px; height: 28px; flex-basis: 28px; background: #9aa3ad">C</span><span style="min-width: 0"><span class="trunc" style="display: block">Controlling</span><span class="trunc small faint mono" style="display: block">Controlling-forge-1.16.5-7.0.0.jar, 88 KB</span></span><button class="btn btn-sm" onclick={() => open(PAGES[1]!)}>Open page</button></div>
  </div>
  <span class="small faint">You can also drop the files onto this window.</span>
  {#snippet foot()}<button class="btn" style="margin-right: auto" onclick={() => session.close()}>Skip these mods</button><button class="btn" onclick={() => PAGES.forEach(open)}>Open all 2 pages</button><button class="btn btn-accent" disabled>Continue</button>{/snippet}
</Dialog>

<style>
  .bf{display:grid;grid-template-columns:28px minmax(0,1fr) auto;gap:12px;align-items:center;min-height:60px;padding:8px 12px}
  @media (prefers-reduced-motion:no-preference){.pulse{animation:pl 1.6s ease-in-out infinite}}
  @keyframes pl{50%{opacity:.35}}
</style>

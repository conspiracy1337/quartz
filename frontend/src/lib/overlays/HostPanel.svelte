<script lang="ts">
  import { copyText } from '../clipboard';
  import { href } from '../router.svelte';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';

  /** Bottom-right panel while a local server starts or runs. */
  interface Props { instanceId: string; state: 'starting' | 'running' }
  let { instanceId, state }: Props = $props();
  const copy = (t: string) => void copyText(t);
</script>

{#if state === 'starting'}
  <section class="panel" role="status" aria-label="Starting server" style="padding: 14px; display: grid; grid-template-columns: 18px minmax(0,1fr); gap: 12px; align-items: center">
    <Icon name="spinner" size={16} sw={2.4} class="spin" color="var(--text)" />
    <span><span style="display: block; font-weight: 500">Starting server</span><span class="small faint">Backing up Northwatch, preparing spawn area</span></span>
  </section>
{:else}
  <section class="panel" aria-label="Server running">
    <div style="display: flex; align-items: center; gap: 10px; padding: 14px 14px 10px"><span class="dot live" aria-hidden="true"></span><span style="font-weight: 500; margin-right: auto">Server running</span><span class="small faint tnum">1 of 10 players</span></div>
    <div style="padding: 0 14px; display: flex; flex-direction: column; gap: 6px">
      <div class="addr"><span class="mono trunc">localhost:25565</span><button class="btn btn-sm btn-ghost" onclick={() => copy('localhost:25565')}>Copy</button></div>
      <div class="addr"><span class="mono trunc">192.168.1.24:25565 <span class="faint">on your network</span></span><button class="btn btn-sm btn-ghost" onclick={() => copy('192.168.1.24:25565')}>Copy</button></div>
    </div>
    <div style="padding: 10px 14px 0">
      <div class="con" aria-label="Server console">[21:40:02] Done (4.218s)! For help, type "help"<br />[21:40:19] Kestrel_Vane joined the game<br />[21:40:31] Saved the game</div>
      <label class="input" style="margin-top: 6px; height: 30px"><span class="mono faint">/</span><span class="sr">Server command</span><input class="mono" placeholder="op Kestrel_Vane" /></label>
    </div>
    <div style="display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 6px; padding: 12px 14px 14px">
      <button class="btn btn-sm btn-accent" onclick={() => session.play(instanceId)}>Join</button>
      <a href={href({ name: 'instance', id: instanceId, tab: 'logs' })} class="btn btn-sm">Console</a>
      <button class="btn btn-sm btn-danger" onclick={() => session.stopHost(instanceId)}>Stop</button>
    </div>
  </section>
{/if}

<style>
  .panel{position:absolute;right:20px;bottom:20px;width:360px;border:1px solid var(--line-2);border-radius:12px;background:var(--raise);box-shadow:0 24px 64px -16px rgba(0,0,0,.8);z-index:10}
  @media (prefers-reduced-motion:no-preference){.panel{animation:in 200ms cubic-bezier(.23,1,.32,1)}}
  @keyframes in{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:none}}
  .addr{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:8px;align-items:center;height:36px;padding:0 6px 0 10px;border:1px solid var(--line);border-radius:8px;background:var(--panel)}
  .con{font-family:var(--mono);font-size:11px;line-height:17px;color:var(--muted);background:#08080a;border:1px solid var(--line);border-radius:8px;padding:8px 10px;height:86px;overflow:hidden}
</style>

<script lang="ts">
  import { api } from '../api';
  import type { HostState } from '../api/host';
  import type { LogLine } from '../api/types';
  import { onEvent } from '../bridge';
  import { query } from '../query.svelte';
  import { href } from '../router.svelte';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';

  /** Bottom-right panel while the local server starts or runs (DlgHost.dc.html). */
  interface Props { host: HostState }
  let { host }: Props = $props();
  const id = $derived(host.instanceId);
  const local = $derived(`localhost:${host.port}`);
  const network = $derived(host.lan ? `${host.lan}:${host.port}` : '');
  const copy = (t: string) => void session.copy(t, 'Address');
  // The command box suggests opping yourself (the design's example, with your name).
  const accounts = query(() => api.accounts.list());
  const me = $derived(accounts.data?.find((a) => a.isDefault)?.name ?? '');

  // The panel shows the newest console lines; the Logs tab has them all.
  const SHOWN = 4;
  let lines = $state.raw<LogLine[]>([]);
  $effect(() => {
    const inst = id;
    let alive = true;
    void api.host.live(inst).then((l) => {
      if (alive) lines = (l ?? []).slice(-SHOWN);
    });
    const off = onEvent<{ instanceId: string; lines: LogLine[] }>('host:log', (b) => {
      if (b.instanceId === inst) lines = lines.concat(b.lines).slice(-SHOWN);
    });
    return () => {
      alive = false;
      off();
    };
  });

  // Lines wrap as on the board, so the box keeps its end in view: the newest output
  // (a command's reply) is never clipped below.
  let con = $state<HTMLElement>();
  $effect(() => {
    void lines;
    if (con) con.scrollTop = con.scrollHeight;
  });

  let command = $state('');
  async function send(e: KeyboardEvent) {
    if (e.key !== 'Enter' || !command.trim()) return;
    try {
      await api.host.command(id, command);
      command = '';
    } catch (err) {
      session.fail('Command', err);
    }
  }
</script>

{#if host.state === 'starting'}
  <section class="panel" role="status" aria-label="Starting server" style="padding: 14px; display: grid; grid-template-columns: 18px minmax(0,1fr); gap: 12px; align-items: center">
    <Icon name="spinner" size={16} sw={2.4} class="spin" color="var(--text)" />
    <span><span style="display: block; font-weight: 500">Starting server</span><span class="small faint">{host.stage ?? ''}</span></span>
  </section>
{:else}
  <section class="panel" aria-label="Server running">
    <div style="display: flex; align-items: center; gap: 10px; padding: 14px 14px 10px"><span class="dot live" aria-hidden="true"></span><span style="font-weight: 500; margin-right: auto">Server running</span><span class="small faint tnum">{host.players} of {host.max} players</span></div>
    <div style="padding: 0 14px; display: flex; flex-direction: column; gap: 6px">
      <div class="addr"><span class="mono trunc">{local}</span><button class="btn btn-sm btn-ghost" onclick={() => copy(local)}>Copy</button></div>
      {#if network}<div class="addr"><span class="mono trunc">{network} <span class="faint">on your network</span></span><button class="btn btn-sm btn-ghost" onclick={() => copy(network)}>Copy</button></div>{/if}
    </div>
    <div style="padding: 10px 14px 0">
      <div class="con" aria-label="Server console" bind:this={con}>{#each lines as l, i (i)}{#if i > 0}<br />{/if}[{l.time}] {l.message}{/each}</div>
      <label class="input" style="margin-top: 6px; height: 30px"><span class="mono faint">/</span><span class="sr">Server command</span><input class="mono" placeholder={me ? `op ${me}` : ''} bind:value={command} onkeydown={send} /></label>
    </div>
    <div style="display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 6px; padding: 12px 14px 14px">
      <button class="btn btn-sm btn-accent" onclick={() => session.play(id, '', { kind: 'server', target: local })}>Join</button>
      <a href={href({ name: 'instance', id, tab: 'logs' })} class="btn btn-sm">Console</a>
      <button class="btn btn-sm btn-danger" onclick={() => session.stopHost(id)}>Stop</button>
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

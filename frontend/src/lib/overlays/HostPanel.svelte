<script lang="ts">
  import { api } from '../api';
  import type { HostState } from '../api/host';
  import type { LogLine } from '../api/types';
  import { onEvent } from '../bridge';
  import { query } from '../query.svelte';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';
  import Players from '../ui/Players.svelte';

  /** Bottom-right panel for the local server (DlgHost.dc.html): it stays after the server
      stops, with Start, until it is closed. */
  interface Props { host: HostState }
  let { host }: Props = $props();
  const id = $derived(host.instanceId);
  const running = $derived(host.state === 'running');
  const RESULT: Record<string, string> = { crash: 'Crashed', failed: "Didn't start", cancelled: 'Cancelled' };
  const aside = $derived(running ? `${host.players} of ${host.max} players` : host.state === 'starting' ? (host.stage ?? '') : (RESULT[host.result ?? ''] ?? 'Stopped'));
  const sample = $derived(running ? host.sample : undefined);
  // The command box suggests opping yourself (the design's example, with your name).
  const accounts = query(() => api.accounts.list(), 'accounts');
  const me = $derived(accounts.data?.find((a) => a.isDefault)?.name ?? '');

  // The console keeps its newest lines; the Logs tab has them all.
  const KEPT = 200;
  let lines = $state.raw<LogLine[]>([]);
  $effect(() => {
    const inst = id;
    let alive = true;
    void api.host.live(inst).then((l) => {
      if (alive) lines = (l ?? []).slice(-KEPT);
    });
    const off = onEvent<{ instanceId: string; lines: LogLine[] }>('host:log', (b) => {
      if (b.instanceId === inst) lines = lines.concat(b.lines).slice(-KEPT);
    });
    return () => {
      alive = false;
      off();
    };
  });

  // Small, the console scrolls; a click opens it up (only the corner icon makes it small
  // again, so lines can be selected). New output keeps the end in view unless the reader
  // has scrolled up.
  let open = $state(false);
  let con = $state<HTMLElement>();
  let pinned = true;
  $effect(() => {
    void lines;
    void open;
    if (con && pinned) con.scrollTop = con.scrollHeight;
  });
  const follow = () => con && (pinned = con.scrollHeight - con.scrollTop - con.clientHeight < 24);

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
  const openFolder = () => api.host.openFolder(id).catch((e) => session.fail('Open folder', e));
</script>

<section class="float panel" aria-label="Server">
  <div style="display: flex; align-items: center; gap: 10px; padding: 12px 10px 10px 14px">
    {#if host.state === 'starting'}<Icon name="spinner" size={14} sw={2.4} class="spin" color="var(--text)" />{:else}<span class="dot" class:live={running} aria-hidden="true"></span>{/if}
    <span style="font-weight: 500; margin-right: auto">{running ? 'Server running' : host.state === 'starting' ? 'Starting server' : 'Server stopped'}</span>
    <button class="btn-bare" aria-label="Open server folder" title="Open server folder" onclick={openFolder}><Icon name="folder" size={15} sw={1.8} /></button>
    <button class="btn-bare" aria-label="Close panel" title="Close" onclick={() => session.closeHostPanel()}><Icon name="x" size={12} sw={2} /></button>
  </div>
  <div class="row1">
    <div class="addr" class:dim={!running}><span class="mono trunc">{host.address}</span><button class="btn btn-sm btn-ghost" disabled={!running} onclick={() => void session.copy(host.address, 'Address')}>Copy</button></div>
    <span class="small faint tnum trunc" style="text-align: right"><Players text={aside} online={host.players} {sample} /></span>
  </div>
  <div style="padding: 10px 14px 0">
    <div class="con-wrap">
      <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
      <div class="con" class:open aria-label="Server console" bind:this={con} onscroll={follow} onclick={() => (open = true)}>{#each lines as l, i (i)}{#if i > 0}<br />{/if}[{l.time}] {l.message}{/each}</div>
      <button class="btn-bare grow" aria-label={open ? 'Show fewer lines' : 'Show more lines'} title={open ? 'Show fewer lines' : 'Show more lines'} onclick={() => (open = !open)}><Icon name={open ? 'collapse' : 'expand'} size={13} sw={1.8} /></button>
    </div>
    <label class="input" style="margin-top: 6px; height: 30px" class:dim={!running}><span class="mono faint">/</span><span class="sr">Server command</span><input class="mono" placeholder={me ? `op ${me}` : ''} bind:value={command} onkeydown={send} disabled={!running} /></label>
  </div>
  <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 6px; padding: 12px 14px 14px">
    <button class="btn btn-sm btn-accent" disabled={host.state !== 'stopped'} onclick={() => session.restartHost(id)}><Icon name="play" size={11} />Start</button>
    <button class="btn btn-sm btn-danger" disabled={host.state === 'stopped'} title={session.hostStopping ? 'Force the server to quit now' : 'Save the world and stop'} onclick={() => session.stopHost(id)}><Icon name={session.hostStopping ? 'skull' : 'stop'} size={10} />{session.hostStopping ? 'Kill' : 'Stop'}</button>
  </div>
</section>

<style>
  .panel{width:360px}
  .row1{padding:0 14px;display:grid;grid-template-columns:220px minmax(0,1fr);gap:12px;align-items:center}
  .addr{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:8px;align-items:center;height:36px;padding:0 6px 0 10px;border:1px solid var(--line);border-radius:8px;background:var(--panel)}
  .dim{opacity:.55}
  .con-wrap{position:relative}
  .con-wrap .grow{position:absolute;top:4px;right:4px;background:var(--console)}
  .con{font-family:var(--mono);font-size:11px;line-height:17px;color:var(--muted);background:var(--console);border:1px solid var(--line);border-radius:8px;padding:8px 26px 8px 10px;height:86px;overflow-y:auto;overscroll-behavior:contain;word-break:break-word;cursor:zoom-in}
  .con.open{height:260px;cursor:auto}
</style>

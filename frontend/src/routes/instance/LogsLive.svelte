<script lang="ts">
  import { tick } from 'svelte';
  import { api } from '$lib/api';
  import type { LiveCopy, LogLevel, LogLine } from '$lib/api/types';
  import { onEvent } from '$lib/bridge';
  import LogsFrame from '$lib/components/LogsFrame.svelte';
  import SearchField from '$lib/ui/SearchField.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import { copyText } from '$lib/clipboard';
  import { since } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import { session } from '$lib/session.svelte';

  interface Props { instanceId: string }
  let { instanceId }: Props = $props();

  // The console keeps the newest lines of one source; Go streams new ones in batches.
  const MAX_LINES = 20_000;
  const WRAPPED_LINES = 2_000;
  const ROW = 20;
  let all = $state.raw<LogLine[]>([]);
  let search = $state('');
  let wrap = $state(true);
  let levels = $state<Record<LogLevel, boolean>>({ INFO: true, WARN: true, ERROR: true, DEBUG: false });
  let con = $state<HTMLElement>();
  let top = $state(0);
  let height = $state(600);
  let follow = true;

  // Sources (LogsD.dc.html): each running copy and, while this instance hosts, its server.
  // The console shows the one picked, else the newest copy, else the server, else the last run.
  const accounts = query(() => api.accounts.list());
  let copies = $state.raw<LiveCopy[]>([]);
  let picked = $state('');
  const hosted = $derived(session.hosting?.instanceId === instanceId ? session.hosting : null);
  const source = $derived(
    (picked === 'server' && hosted) || copies.some((c) => c.runId === picked) ? picked : (copies.at(-1)?.runId ?? (hosted ? 'server' : '')),
  );
  const accountName = (id: string) => accounts.data?.find((a) => a.id === id)?.name ?? 'Unknown account';
  $effect(() => {
    const id = instanceId;
    const load = () => void api.logs.copies(id).then((c) => (copies = c ?? []));
    load();
    return onEvent<{ instanceId: string }>('game:state', (s) => s.instanceId === id && load());
  });

  $effect(() => {
    const id = instanceId;
    const src = source;
    const server = src === 'server';
    let alive = true;
    all = [];
    follow = true;
    void (server ? api.host.live(id) : api.logs.live(id, src)).then((l) => {
      if (alive) void append(l ?? [], true);
    });
    const offs = [
      onEvent<{ instanceId: string; runId: string; lines: LogLine[] }>(server ? 'host:log' : 'game:log', (b) => {
        if (b.instanceId === id && (server || b.runId === src)) void append(b.lines, false);
      }),
      // A restarted server starts a fresh console.
      onEvent<{ instanceId: string; state: string }>('host:state', (s) => {
        if (server && s.instanceId === id && s.state === 'starting') all = [];
      }),
    ];
    return () => {
      alive = false;
      offs.forEach((off) => off());
    };
  });

  async function append(lines: LogLine[], replace: boolean) {
    const next = replace ? lines : all.concat(lines);
    all = next.length > MAX_LINES ? next.slice(next.length - MAX_LINES) : next;
    if (follow && con) {
      await tick();
      con.scrollTop = con.scrollHeight;
    }
  }
  function scrolled() {
    if (!con) return;
    top = con.scrollTop;
    height = con.clientHeight;
    follow = con.scrollHeight - con.scrollTop - con.clientHeight < ROW * 2;
  }
  async function copy() {
    await copyText(shown.map((l) => `[${l.time}] [${l.thread ? l.thread + '/' : ''}${l.level}]: ${l.message}`).join('\n'));
  }
  const count = (l: LogLevel) => all.filter((x) => x.level === l).length;
  const shown = $derived(all.filter((l) => levels[l.level]));
  // Wrapped rows have no fixed height, so that view shows the newest lines only;
  // unwrapped rows are 20 px each and only the visible window is rendered.
  const wrapped = $derived(shown.length > WRAPPED_LINES ? shown.slice(shown.length - WRAPPED_LINES) : shown);
  const first = $derived(wrap ? 0 : Math.max(0, Math.floor(top / ROW) - 20));
  const last = $derived(wrap ? wrapped.length : Math.min(shown.length, Math.ceil((top + height) / ROW) + 20));
  const visible = $derived(wrap ? wrapped : shown.slice(first, last));
  const LEVEL_CHIPS: { level: LogLevel; color?: string }[] = [{ level: 'INFO' }, { level: 'WARN', color: 'var(--warn)' }, { level: 'ERROR', color: 'var(--danger)' }, { level: 'DEBUG' }];
  const label = (l: LogLevel) => l.charAt(0) + l.slice(1).toLowerCase();

  /** Splits a message around the first search match, case-insensitively. */
  function parts(msg: string): [string, string, string] {
    const q = search.trim().toLowerCase();
    const at = q ? msg.toLowerCase().indexOf(q) : -1;
    return at < 0 ? [msg, '', ''] : [msg.slice(0, at), msg.slice(at, at + q.length), msg.slice(at + q.length)];
  }
</script>

<LogsFrame {instanceId} active="live">
  {#snippet tools()}
    <SearchField bind:value={search} label="Search log" placeholder="Search log" style="flex: 0 1 240px; min-width: 140px" />
    <div style="display: flex; gap: 6px">
      {#each LEVEL_CHIPS as c (c.level)}
        {@const n = count(c.level)}
        <button class="chip" class:on={levels[c.level]} style={levels[c.level] && c.color ? `background: ${c.color}; border-color: ${c.color}` : undefined} onclick={() => (levels[c.level] = !levels[c.level])}>{label(c.level)}{c.color && n ? ` ${n}` : ''}</button>
      {/each}
    </div>
    <div style="flex: 1 1 0"></div>
    <button class="btn btn-sm" onclick={copy}>Copy</button>
    <button class="btn btn-sm" aria-pressed={wrap} onclick={() => (wrap = !wrap)}>Wrap</button>
    <button class="btn btn-sm" disabled={source === 'server'} title={source === 'server' ? 'Server logs stay on this computer' : undefined} onclick={() => session.upload(() => api.logs.uploadRun(instanceId, source))}>Upload to mclo.gs</button>
  {/snippet}
  {#snippet rail()}
    <div class="side-label">Running</div>
    {#each copies as c, k (c.runId)}
      <button class="rail-row" class:on={source === c.runId} aria-pressed={source === c.runId} onclick={() => (picked = c.runId)}>
        <Icon name="cube" size={14} /><span style="min-width: 0"><span class="trunc" style="display: block">Copy {k + 1}</span><span class="small faint trunc" style="display: block">{accountName(c.accountId)}, {since(c.started)}</span></span><span class="small faint">Game</span>
      </button>
    {/each}
    {#if hosted}
      <button class="rail-row" class:on={source === 'server'} aria-pressed={source === 'server'} onclick={() => (picked = 'server')}>
        <Icon name="server" size={14} /><span style="min-width: 0"><span class="trunc" style="display: block">Server</span><span class="small faint trunc" style="display: block">{hosted.state === 'running' ? `localhost:${hosted.port}` : 'Starting'}</span></span><span class="small faint">Server</span>
      </button>
    {/if}
    {#if !copies.length && !hosted}<span class="small faint" style="padding: 6px 10px">Nothing is running. The last run's log shows here.</span>{/if}
  {/snippet}
  <div class="con" class:nowrap={!wrap} role="log" aria-label="Game output" bind:this={con} onscroll={scrolled}>
    {#if !wrap}<div style="height: {first * ROW}px" aria-hidden="true"></div>{/if}
    {#each visible as l, i (first + i)}
      {@const [pre, mark, post] = parts(l.message)}
      <div class="ln {l.level}"><span class="t">{l.time}</span><span class="lv-{l.level}">{l.level}</span><span>{#if l.thread}<span class="faint">[{l.thread}]</span>{' '}{/if}{pre}{#if mark}<mark>{mark}</mark>{/if}{post}{#if l.redacted}<span class="red">redacted</span>{/if}</span></div>
    {/each}
    {#if !wrap}<div style="height: {(shown.length - last) * ROW}px" aria-hidden="true"></div>{/if}
  </div>
</LogsFrame>

<style>
  .con{flex:1 1 auto;min-height:0;overflow:auto;border:1px solid var(--line);border-radius:10px;background:#08080a;font-family:var(--mono);font-size:12px;line-height:20px;padding:8px 0}
  .ln{display:grid;grid-template-columns:72px 44px minmax(0,1fr);gap:12px;padding:0 14px;white-space:pre-wrap}
  .nowrap .ln{white-space:pre;height:20px;overflow:hidden}
  .ln:hover{background:#111114}
  .ln .t{color:var(--faint)}
  .lv-INFO{color:var(--muted)} .lv-WARN{color:var(--warn)} .lv-ERROR{color:var(--danger)} .lv-DEBUG{color:var(--faint)}
  .ln.ERROR{background:color-mix(in srgb,var(--danger) 7%,transparent)}
  .red{background:var(--raise);color:var(--faint);border-radius:3px;padding:0 4px}
  mark{background:color-mix(in srgb,var(--accent) 30%,transparent);color:var(--text);border-radius:2px}
</style>

<script lang="ts">
  import { tick } from 'svelte';
  import { api } from '$lib/api';
  import type { LogLevel, LogLine } from '$lib/api/types';
  import { onEvent } from '$lib/bridge';
  import LogsNav from '$lib/components/LogsNav.svelte';
  import SearchField from '$lib/ui/SearchField.svelte';
  import { copyText } from '$lib/clipboard';
  import { session } from '$lib/session.svelte';

  interface Props { instanceId: string }
  let { instanceId }: Props = $props();

  // The console keeps the newest lines of the current run; Go streams new ones in batches.
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

  $effect(() => {
    const id = instanceId;
    let alive = true;
    void api.logs.live(id).then((l) => {
      if (alive) void append(l ?? [], true);
    });
    const offLog = onEvent<{ instanceId: string; lines: LogLine[] }>('game:log', (b) => {
      if (b.instanceId === id) void append(b.lines, false);
    });
    const offState = onEvent<{ instanceId: string; state: string }>('game:state', (s) => {
      if (s.instanceId === id && s.state === 'running') all = [];
    });
    return () => {
      alive = false;
      offLog();
      offState();
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

<div style="display: flex; flex-wrap: wrap; gap: 8px; align-items: center; padding: 14px 28px 12px">
  <LogsNav {instanceId} active="live" />
  <div style="display: flex; gap: 6px">
    {#each LEVEL_CHIPS as c (c.level)}
      {@const n = count(c.level)}
      <button class="chip" class:on={levels[c.level]} style={levels[c.level] && c.color ? `background: ${c.color}; border-color: ${c.color}` : undefined} onclick={() => (levels[c.level] = !levels[c.level])}>{label(c.level)}{c.color && n ? ` ${n}` : ''}</button>
    {/each}
  </div>
  <SearchField bind:value={search} label="Search log" style="flex: 1 1 160px; max-width: 240px" />
  <div style="flex: 1 1 0"></div>
  <button class="btn btn-sm" aria-pressed={wrap} onclick={() => (wrap = !wrap)}>Wrap</button>
  <button class="btn btn-sm" onclick={copy}>Copy</button>
  <button class="btn btn-sm" onclick={() => session.upload(() => api.logs.uploadRun(instanceId, ''))}>Upload to mclo.gs</button>
</div>
<div class="con" class:nowrap={!wrap} role="log" aria-label="Game output" bind:this={con} onscroll={scrolled}>
  {#if !wrap}<div style="height: {first * ROW}px" aria-hidden="true"></div>{/if}
  {#each visible as l, i (first + i)}
    {@const [pre, mark, post] = parts(l.message)}
    <div class="ln {l.level}"><span class="t">{l.time}</span><span class="lv-{l.level}">{l.level}</span><span>{#if l.thread}<span class="faint">[{l.thread}]</span>{' '}{/if}{pre}{#if mark}<mark>{mark}</mark>{/if}{post}{#if l.redacted}<span class="red">redacted</span>{/if}</span></div>
  {/each}
  {#if !wrap}<div style="height: {(shown.length - last) * ROW}px" aria-hidden="true"></div>{/if}
</div>

<style>
  .con{flex:1 1 auto;min-height:0;overflow:auto;margin:0 28px 20px;border:1px solid var(--line);border-radius:10px;background:#08080a;font-family:var(--mono);font-size:12px;line-height:20px;padding:8px 0}
  .ln{display:grid;grid-template-columns:72px 44px minmax(0,1fr);gap:12px;padding:0 14px;white-space:pre-wrap}
  .nowrap .ln{white-space:pre;height:20px;overflow:hidden}
  .ln:hover{background:#111114}
  .ln .t{color:var(--faint)}
  .lv-INFO{color:var(--muted)} .lv-WARN{color:var(--warn)} .lv-ERROR{color:var(--danger)} .lv-DEBUG{color:var(--faint)}
  .ln.ERROR{background:color-mix(in srgb,var(--danger) 7%,transparent)}
  .red{background:var(--raise);color:var(--faint);border-radius:3px;padding:0 4px}
  mark{background:color-mix(in srgb,var(--accent) 30%,transparent);color:var(--text);border-radius:2px}
</style>

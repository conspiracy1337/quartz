<script lang="ts">
  import { api } from '$lib/api';
  import type { LogLevel } from '$lib/api/types';
  import LogsNav from '$lib/components/LogsNav.svelte';
  import { query } from '$lib/query.svelte';
  import SearchField from '$lib/ui/SearchField.svelte';

  interface Props { instanceId: string }
  let { instanceId }: Props = $props();

  const lines = query(() => api.logs.live(instanceId));
  let search = $state('mixin');
  let wrap = $state(true);
  let levels = $state<Record<LogLevel, boolean>>({ INFO: true, WARN: true, ERROR: true, DEBUG: false });

  const all = $derived(lines.data ?? []);
  const count = (l: LogLevel) => all.filter((x) => x.level === l).length;
  const shown = $derived(all.filter((l) => levels[l.level]));
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
  <button class="btn btn-sm">Copy</button>
  <button class="btn btn-sm">Upload to mclo.gs</button>
</div>
<div class="con" class:nowrap={!wrap} role="log" aria-label="Game output">
  {#each shown as l, i (i)}
    {@const [pre, mark, post] = parts(l.message)}
    <div class="ln {l.level}"><span class="t">{l.time}</span><span class="lv-{l.level}">{l.level}</span><span><span class="faint">[{l.thread}]</span> {pre}{#if mark}<mark>{mark}</mark>{/if}{post}{#if l.redacted}<span class="red">redacted</span>{/if}</span></div>
  {/each}
</div>

<style>
  .con{flex:1 1 auto;min-height:0;overflow:auto;margin:0 28px 20px;border:1px solid var(--line);border-radius:10px;background:#08080a;font-family:var(--mono);font-size:12px;line-height:20px;padding:8px 0}
  .ln{display:grid;grid-template-columns:72px 44px minmax(0,1fr);gap:12px;padding:0 14px;white-space:pre-wrap}
  .nowrap .ln{white-space:pre}
  .ln:hover{background:#111114}
  .ln .t{color:var(--faint)}
  .lv-INFO{color:var(--muted)} .lv-WARN{color:var(--warn)} .lv-ERROR{color:var(--danger)} .lv-DEBUG{color:var(--faint)}
  .ln.ERROR{background:color-mix(in srgb,var(--danger) 7%,transparent)}
  .red{background:var(--raise);color:var(--faint);border-radius:3px;padding:0 4px}
  mark{background:color-mix(in srgb,var(--accent) 30%,transparent);color:var(--text);border-radius:2px}
</style>

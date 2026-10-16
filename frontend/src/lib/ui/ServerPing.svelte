<script lang="ts">
  /** A server's ping: milliseconds, then four bars that fill as it gets faster. */
  interface Props { ms: number | undefined }
  let { ms }: Props = $props();
  const level = $derived(ms === undefined ? '' : ms < 30 ? 'p4' : ms < 80 ? 'p3' : ms < 250 ? 'p2' : 'p1');
</script>

<span class="s-ping"><span class="s-ms">{ms !== undefined ? `${ms} ms` : ''}</span><span class="ping {level}" aria-hidden="true"><i style="height: 4px"></i><i style="height: 6px"></i><i style="height: 8px"></i><i style="height: 11px"></i></span></span>

<style>
  .s-ping{display:flex;align-items:center;justify-content:flex-end;gap:8px}
  .s-ms{text-align:right;font-family:var(--mono);font-size:12px;color:var(--muted);font-variant-numeric:tabular-nums;white-space:nowrap}
  .ping{display:inline-flex;align-items:flex-end;gap:2px;height:11px}
  .ping i{width:3px;background:var(--line-2);border-radius:1px}
  .ping.p4 i,.ping.p3 i:nth-child(-n+3),.ping.p2 i:nth-child(-n+2),.ping.p1 i:nth-child(1){background:var(--accent)}
</style>

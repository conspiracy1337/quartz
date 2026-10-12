<script lang="ts">
  import { api } from '$lib/api';
  import LogsNav from '$lib/components/LogsNav.svelte';
  import { bytes } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';

  interface Props { instanceId: string }
  let { instanceId }: Props = $props();

  const files = query(() => api.logs.files(instanceId));
  let selected = $state('latest.log');
  const text = query(() => api.logs.text(instanceId, selected));
  let find = $state('');
  const logs = $derived((files.data ?? []).filter((f) => !f.crash));
  const crashes = $derived((files.data ?? []).filter((f) => f.crash));
  const lines = $derived((text.data ?? '').split('\n'));
  const tone = (l: string) => (l.includes('/ERROR]') ? 'var(--danger)' : l.includes('/WARN]') ? 'var(--warn)' : undefined);
</script>

<div class="lb">
  <nav aria-label="Log files" class="scroll" style="border-right: 1px solid var(--line); padding: 14px 10px; display: flex; flex-direction: column; gap: 2px">
    <LogsNav {instanceId} active="files" style="margin: 0 2px 10px; width: max-content" />
    <div class="side-label">Logs</div>
    {#each logs as f (f.name)}
      <button class="fl" class:on={selected === f.name} onclick={() => (selected = f.name)}><Icon name="file" size={14} /><span class="trunc mono">{f.name}</span><span class="small faint">{bytes(f.bytes)}</span></button>
    {/each}
    <div class="side-label" style="margin-top: 10px">Crash reports</div>
    {#each crashes as f (f.name)}
      <button class="fl" onclick={() => session.open({ kind: 'crash', instanceId })}><Icon name="warn" size={14} color="var(--danger)" /><span class="trunc mono">{f.name}</span><span class="small faint">{bytes(f.bytes)}</span></button>
    {/each}
    <div style="margin-top: auto; padding-top: 12px"><button class="btn btn-sm btn-block">Delete old logs, 1.4 MB</button></div>
  </nav>
  <section aria-label={selected} style="display: flex; flex-direction: column; min-height: 0; min-width: 0">
    <div style="display: flex; gap: 8px; align-items: center; padding: 12px 20px; border-bottom: 1px solid var(--line)">
      <h2 class="h3 mono trunc" style="margin-right: auto">{selected}</h2>
      <label class="input" style="width: 200px; height: 28px"><span class="sr">Find</span><input type="search" placeholder="Find" bind:value={find} /></label>
      <button class="btn btn-sm">Copy</button>
      <button class="btn btn-sm">Upload</button>
    </div>
    <div class="scroll" style="padding: 12px 20px">
      <pre class="code">{#each lines as l, i (i)}{#if tone(l)}<span style="color: {tone(l)}">{l}</span>{:else}{l}{/if}{'\n'}{/each}</pre>
    </div>
  </section>
</div>

<style>
  .lb{display:grid;grid-template-columns:260px minmax(0,1fr);flex:1 1 auto;min-height:0}
  @media (max-width:1100px){.lb{grid-template-columns:220px minmax(0,1fr)}}
  .fl{display:grid;grid-template-columns:16px minmax(0,1fr) auto;gap:10px;align-items:center;height:40px;padding:0 10px;border-radius:6px;border:0;background:transparent;width:100%;text-align:left;color:var(--muted)}
  .fl:hover{background:var(--raise);color:var(--text)}
  .fl.on{background:var(--hover);color:var(--text)}
  .code{font-family:var(--mono);font-size:12px;line-height:20px;white-space:pre-wrap;color:var(--muted);margin:0}
</style>

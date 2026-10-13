<script lang="ts">
  import { api } from '$lib/api';
  import LogsNav from '$lib/components/LogsNav.svelte';
  import { bytes } from '$lib/format';
  import { revs } from '$lib/revs.svelte';
  import { query } from '$lib/query.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Empty from '$lib/ui/Empty.svelte';

  interface Props { instanceId: string }
  let { instanceId }: Props = $props();

  const files = query(() => api.logs.files(instanceId));
  // A run's saved file opened from Launch history, else a file of the logs folder.
  // svelte-ignore state_referenced_locally
  const opened = session.runFile?.instanceId === instanceId ? session.runFile : null;
  session.runFile = null;
  let run = $state(opened);
  let selected = $state(opened ? opened.title : 'latest.log');
  const text = query(() => (run ? api.logs.runText(instanceId, run.runId, run.kind) : api.logs.text(instanceId, selected)));
  const old = query(() => revs.load('instances', () => api.logs.oldSize(instanceId)));
  function pick(name: string) {
    run = null;
    selected = name;
  }
  async function deleteOld() {
    try {
      const freed = await api.logs.deleteOld(instanceId);
      revs.bump('instances');
      session.notify({ subject: `${bytes(freed)} freed`, text: 'Old logs were deleted.' });
    } catch (e) {
      session.fail('Delete old logs', e);
    }
  }
  const copy = () => session.copy(text.data ?? '', selected);
  const upload = () => session.upload(() => (run ? api.logs.uploadRun(instanceId, run.runId) : api.logs.uploadFile(instanceId, selected)));
  /** Splits a line around the first match of Find, case-insensitively (as the live console does). */
  function parts(line: string): [string, string, string] {
    const q = find.trim().toLowerCase();
    const at = q ? line.toLowerCase().indexOf(q) : -1;
    return at < 0 ? [line, '', ''] : [line.slice(0, at), line.slice(at, at + q.length), line.slice(at + q.length)];
  }
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
      <button class="fl" class:on={!run && selected === f.name} onclick={() => pick(f.name)}><Icon name="file" size={14} /><span class="trunc mono">{f.name}</span><span class="small faint">{bytes(f.bytes)}</span></button>
    {:else}
      {#if files.data}<span class="small faint" style="padding: 6px 8px">None yet</span>{/if}
    {/each}
    <div class="side-label" style="margin-top: 10px">Crash reports</div>
    {#each crashes as f (f.name)}
      <button class="fl" onclick={() => session.open({ kind: 'crash', instanceId, file: f.name })}><Icon name="warn" size={14} color="var(--danger)" /><span class="trunc mono">{f.name}</span><span class="small faint">{bytes(f.bytes)}</span></button>
    {:else}
      {#if files.data}<span class="small faint" style="padding: 6px 8px">None</span>{/if}
    {/each}
    <div style="margin-top: auto; padding-top: 12px"><button class="btn btn-sm btn-block" disabled={!old.data} onclick={deleteOld}>Delete old logs{old.data ? `, ${bytes(old.data)}` : ''}</button></div>
  </nav>
  <section aria-label={selected} style="display: flex; flex-direction: column; min-height: 0; min-width: 0">
    <div style="display: flex; gap: 8px; align-items: center; padding: 12px 20px; border-bottom: 1px solid var(--line)">
      <h2 class="h3 mono trunc" style="margin-right: auto">{selected}</h2>
      <label class="input" style="width: 200px; height: 28px"><span class="sr">Find</span><input type="search" placeholder="Find" bind:value={find} /></label>
      <button class="btn btn-sm" disabled={!text.data} onclick={copy}>Copy</button>
      <button class="btn btn-sm" disabled={!text.data} onclick={upload}>Upload</button>
    </div>
    <div class="scroll" style="padding: 12px 20px">
      {#if text.data === ''}<Empty text="Log files appear here after you play." />{:else}<pre class="code">{#each lines as l, i (i)}{@const [pre, mark, post] = parts(l)}<span style:color={tone(l)}>{pre}{#if mark}<mark>{mark}</mark>{/if}{post}</span>{'\n'}{/each}</pre>{/if}
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
  .code mark{background:color-mix(in srgb,var(--warn) 30%,transparent);color:var(--text);border-radius:2px}
</style>

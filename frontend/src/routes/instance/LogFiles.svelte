<script lang="ts">
  import { api } from '$lib/api';
  import LogsFrame from '$lib/components/LogsFrame.svelte';
  import { bytes } from '$lib/format';
  import { revs } from '$lib/revs.svelte';
  import { query, skeletonRows } from '$lib/query.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Empty from '$lib/ui/Empty.svelte';

  interface Props { instanceId: string }
  let { instanceId }: Props = $props();

  // Keyed, so a second visit lists the files at once (the first shows as many skeleton rows as last time).
  // The instance page is remounted for each instance, so the key follows it.
  // svelte-ignore state_referenced_locally
  const files = query(() => revs.load('instances', () => api.logs.files(instanceId)), `logs.files.${instanceId}`);
  // A run's saved file opened from Launch history, else a file of the logs folder.
  // svelte-ignore state_referenced_locally
  const opened = session.runFile?.instanceId === instanceId ? session.runFile : null;
  session.runFile = null;
  let run = $state(opened);
  // The file picked, else the first one there is (an instance that never played has none).
  let picked = $state('');
  const logs = $derived((files.data ?? []).filter((f) => !f.crash));
  const crashes = $derived((files.data ?? []).filter((f) => f.crash));
  const selected = $derived(logs.some((f) => f.name === picked) ? picked : (logs[0]?.name ?? ''));
  const title = $derived(run ? run.title : selected);
  const text = query(() => (run ? api.logs.runText(instanceId, run.runId, run.kind) : selected ? api.logs.text(instanceId, selected) : Promise.resolve('')));
  const old = query(() => revs.load('instances', () => api.logs.oldSize(instanceId)));
  function pick(name: string) {
    run = null;
    picked = name;
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
  const copy = () => session.copy(text.data ?? '', title);
  const upload = () => session.upload(() => (run ? api.logs.uploadRun(instanceId, run.runId) : api.logs.uploadFile(instanceId, selected)));
  /** Splits a line around the first match of Find, case-insensitively (as the live console does). */
  function parts(line: string): [string, string, string] {
    const q = find.trim().toLowerCase();
    const at = q ? line.toLowerCase().indexOf(q) : -1;
    return at < 0 ? [line, '', ''] : [line.slice(0, at), line.slice(at, at + q.length), line.slice(at + q.length)];
  }
  let find = $state('');
  const lines = $derived((text.data ?? '').split('\n'));
  const tone = (l: string) => (l.includes('/ERROR]') ? 'var(--danger)' : l.includes('/WARN]') ? 'var(--warn)' : undefined);
</script>

{#snippet skRow(k: number)}
  <div class="rail-row" aria-hidden="true"><span class="sk" style="width: 14px; height: 14px"></span><span class="sk" style="height: 12px; width: {[70, 56, 64][k % 3]}%"></span><span class="sk" style="height: 10px; width: 40px"></span></div>
{/snippet}

<LogsFrame {instanceId} active="files">
  {#snippet tools()}
    <div style="flex: 1 1 0"></div>
    <!-- Wide enough for its size, so the button keeps its place when the size comes in. -->
    <button class="btn btn-sm" style="min-width: 160px" disabled={!old.data} onclick={deleteOld}>Delete old logs{old.data ? `, ${bytes(old.data)}` : ''}</button>
  {/snippet}
  {#snippet rail()}
    <div class="side-label">Logs</div>
    {#each logs as f (f.name)}
      <button class="rail-row" class:on={!run && selected === f.name} onclick={() => pick(f.name)}><Icon name="file" size={14} /><span class="trunc mono">{f.name}</span><span class="small faint">{bytes(f.bytes)}</span></button>
    {:else}
      {#if files.data}<span class="small faint" style="padding: 6px 10px">None yet</span>
      {:else}{#each { length: skeletonRows(`logs.files.${instanceId}`, 3) } as _, k (k)}{@render skRow(k)}{/each}{/if}
    {/each}
    <div class="side-label" style="margin-top: 10px">Crash reports</div>
    {#each crashes as f (f.name)}
      <button class="rail-row" onclick={() => session.open({ kind: 'crash', instanceId, file: f.name })}><Icon name="warn" size={14} color="var(--danger)" /><span class="trunc mono">{f.name}</span><span class="small faint">{bytes(f.bytes)}</span></button>
    {:else}
      {#if files.data}<span class="small faint" style="padding: 6px 10px">None</span>{:else}{@render skRow(0)}{/if}
    {/each}
  {/snippet}
  <section class="lv" aria-label={title || 'Log file'}>
    <div class="lv-head">
      <h2 class="h3 mono trunc" style="margin-right: auto">{title}</h2>
      <label class="input" style="width: 200px; height: 28px"><span class="sr">Find</span><input type="search" placeholder="Find" bind:value={find} /></label>
      <button class="btn btn-sm" disabled={!text.data} onclick={copy}>Copy</button>
      <button class="btn btn-sm" disabled={!text.data || run?.kind === 'cmd'} onclick={upload}>Upload</button>
    </div>
    <div class="scroll" style="padding: 12px 16px">
      {#if text.error}<Empty text="This file couldn't be read. It may have been deleted." />
      {:else if text.data === ''}<Empty text="Log files appear here after you play." />
      {:else if text.data}<pre class="code">{#each lines as l, i (i)}{@const [pre, mark, post] = parts(l)}<span style:color={tone(l)}>{pre}{#if mark}<mark>{mark}</mark>{/if}{post}</span>{'\n'}{/each}</pre>{/if}
    </div>
  </section>
</LogsFrame>

<style>
  /* The file, in a box like the live console. */
  .lv{flex:1 1 auto;min-height:0;display:flex;flex-direction:column;border:1px solid var(--line);border-radius:10px;background:var(--panel);overflow:hidden}
  .lv-head{display:flex;gap:8px;align-items:center;padding:10px 16px;border-bottom:1px solid var(--line)}
  .code{font-family:var(--mono);font-size:12px;line-height:20px;white-space:pre-wrap;color:var(--muted);margin:0}
  .code mark{background:color-mix(in srgb,var(--warn) 30%,transparent);color:var(--text);border-radius:2px}
</style>

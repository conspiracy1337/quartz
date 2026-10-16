<script lang="ts">
  import { api } from '../api';
  import type { ExportCode, ExportEntry, ExportFormat, ExportInclude, ExportListFormat, ExportMeta, ExportPreview } from '../api/types';
  import { copyText } from '../clipboard';
  import { bytes, errorText, shortMonthDay } from '../format';
  import { SRC_NAMES } from '../labels';
  import { artOf } from '../look';
  import { query } from '../query.svelte';
  import { session } from '../session.svelte';
  import { tasks } from '../tasks.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Icon from '../ui/Icon.svelte';
  import Seg from '../ui/Seg.svelte';
  import Tile from '../ui/Tile.svelte';
  import ToggleRow from '../ui/ToggleRow.svelte';

  /** Export an instance (board DlgExport, variant C: four summary numbers; one opens its files below). */
  interface Props { instanceId: string }
  let { instanceId }: Props = $props();

  const FORMATS = [
    { value: 'list', label: 'Mod list' },
    { value: 'mrpack', label: 'Modrinth' },
    { value: 'curseforge', label: 'CurseForge' },
    { value: 'zip', label: 'Full zip' },
  ] as const;
  const LISTS = [
    { value: 'md', label: 'Markdown' },
    { value: 'json', label: 'JSON' },
    { value: 'code', label: 'Code' },
  ] as const;
  const LIST_HELP: Record<ExportListFormat, string> = {
    md: 'A readable table of names, versions and links.',
    json: 'A file other tools and launchers can read back.',
    code: 'A short code for this list, kept online for 7 days.',
  };
  const PARTS: { key: keyof ExportInclude; label: string }[] = [
    { key: 'options', label: 'Options and keybinds' },
    { key: 'servers', label: 'Server list' },
    { key: 'config', label: 'Mod settings' },
    { key: 'worlds', label: 'Worlds' },
    { key: 'screenshots', label: 'Screenshots' },
  ];

  const inst = query(() => api.instances.get(instanceId));
  let format = $state<ExportFormat>('list');
  let list = $state<ExportListFormat>('md');
  let include = $state<ExportInclude>({ options: true, servers: true, config: true, worlds: false, screenshots: false });
  let busy = $state(false);
  let code = $state<ExportCode | null>(null);
  let copied = $state(false);
  /** The summary number whose files show below (null: the first). */
  let stat = $state<keyof ExportPreview | null>(null);
  /** The pack's name, version and summary (Prism's export fields); never the instance's private notes. */
  let meta = $state<ExportMeta>({ name: '', version: '' });
  // svelte-ignore state_referenced_locally
  void api.exports.meta(instanceId).then((m) => (meta = { summary: '', author: '', ...m }));

  const plan = $derived({ format, list, include: { ...include } });
  const preview = query(() => api.exports.preview(instanceId, plan));
  const isList = $derived(format === 'list');
  const sharing = $derived(isList && list === 'code');

  type Cell = { key: keyof ExportPreview; label: string; warn?: boolean; right: (e: ExportEntry) => string };
  const size = (e: ExportEntry) => bytes(e.bytes);
  const from = (e: ExportEntry) => (e.source ? SRC_NAMES[e.source] : '');
  /** The numbers each format sums up, in order; the last box is the export's size. */
  const cells = $derived.by((): Cell[] => {
    const off: Cell = { key: 'off', label: 'Left out (off)', right: size };
    if (isList) return [{ key: 'listed', label: 'In the list', right: from }, { key: 'unlisted', label: 'Not listed', warn: true, right: size }, off];
    const bundled: Cell = { key: 'bundled', label: 'Bundled', right: size };
    if (format === 'zip') return [bundled, off];
    return [{ key: 'linked', label: `From ${format === 'mrpack' ? 'Modrinth' : 'CurseForge'}`, right: from }, bundled, off];
  });
  const open = $derived(cells.find((c) => c.key === stat) ?? cells[0]!);
  const rows = $derived((preview.data?.[open.key] ?? []) as ExportEntry[]);
  const unlisted = $derived(isList ? (preview.data?.unlisted.length ?? 0) : 0);

  function pickFormat() {
    stat = null;
    code = null;
  }

  async function run() {
    if (busy) return;
    if (code) return session.close();
    busy = true;
    try {
      const res = await api.exports.run(instanceId, { ...plan, meta: { ...meta } });
      if (res.code) {
        code = res.code;
        return;
      }
      if (!res.job && !res.file) return; // the save dialog was cancelled
      session.close();
      if (res.job) {
        await tasks.load();
        session.notify({ subject: `Exporting ${inst.data?.name ?? 'the instance'}`, text: res.file ?? '' });
      } else {
        session.notify({ subject: 'Mod list saved', text: res.file ?? '' });
      }
    } catch (e) {
      session.fail(sharing ? 'Create a code' : 'Export', e);
    } finally {
      busy = false;
    }
  }

  async function copy() {
    if (code) copied = await copyText(code.code);
  }
</script>

<Dialog title="Export {inst.data?.name ?? 'instance'}" sub="Share it, or keep a copy." width={520} onclose={() => session.close()}>
  <div class="field"><span class="field-label">Format</span><Seg options={FORMATS} bind:value={format} onchange={pickFormat} label="Format" fill center /></div>
  {#if isList}
    <div class="field"><span class="field-label">File type</span><Seg options={LISTS} bind:value={list} onchange={pickFormat} label="File type" fill center /><span class="field-help">{LIST_HELP[list]}</span></div>
    {#if code}
      <div class="field"><span class="field-label">Your code</span>
        <div class="codebox">
          <span class="code">{code.code}</span>
          <span class="small faint" style="flex: 1 1 auto">Valid for 7 days, until {shortMonthDay(code.expiresAt)}. Anyone with it can load this mod list.</span>
          <button class="btn btn-sm" onclick={copy}><Icon name={copied ? 'check' : 'link'} size={14} />{copied ? 'Copied' : 'Copy'}</button>
        </div>
      </div>
    {/if}
  {:else}
    <div style="display: grid; grid-template-columns: minmax(0,1fr) 120px; gap: 12px">
      <label class="field"><span class="field-label">Name</span><span class="input"><input bind:value={meta.name} maxlength="64" placeholder={inst.data?.name} /></span></label>
      <label class="field"><span class="field-label">Version</span><span class="input"><input bind:value={meta.version} maxlength="32" placeholder="1.0.0" /></span></label>
    </div>
    {#if format === 'curseforge'}
      <label class="field"><span class="field-label">Author</span><span class="input"><input bind:value={meta.author} maxlength="64" placeholder="Optional" /></span></label>
    {:else}
      <label class="field"><span class="field-label">Summary</span><span class="input"><input bind:value={meta.summary} maxlength="500" placeholder="Optional" /></span></label>
    {/if}
    <div class="field"><span class="field-label">Include</span>
      <div class="card tgl-grid">
        {#each PARTS as part (part.key)}<ToggleRow name={part.label} on={include[part.key]} onchange={(v) => (include[part.key] = v)} />{/each}
      </div>
    </div>
  {/if}
  <div class="field"><span class="field-label">What's in it</span>
    {#if preview.error}
      <span class="field-help">{errorText(preview.error)}</span>
    {:else}
      {@const pv = preview.data}
      <div class="card stats" style="grid-template-columns: repeat({cells.length + 1}, minmax(0,1fr))">
        {#each cells as c (c.key)}
          {@const n = (pv?.[c.key] as ExportEntry[] | undefined)?.length}
          <button class="stat" class:on={open.key === c.key} aria-pressed={open.key === c.key} onclick={() => (stat = c.key)}>
            {#if n === undefined}<span class="sk" style="height: 16px; width: 24px; margin: 3px 0"></span>{:else}<span class="n tnum" style:color={c.warn && n ? 'var(--warn)' : undefined}>{n}</span>{/if}
            <span class="small faint trunc" title={c.label}>{c.label}</span>
          </button>
        {/each}
        <div class="stat" aria-label="Size">
          {#if !pv}<span class="sk" style="height: 16px; width: 48px; margin: 3px 0"></span>{:else}<span class="n tnum">{sharing ? '7 days' : bytes(pv.bytes)}</span>{/if}
          <span class="small faint trunc">{sharing ? 'Code lasts' : isList ? 'File size' : 'Export size'}</span>
        </div>
      </div>
      <div class="list scroll" style="max-height: 216px">
        {#each rows as e (e.name + e.detail)}
          <div class="frow"><Tile art={artOf(e.name)} size="sm" /><span class="trunc">{e.name}{#if e.detail}<span class="faint small" style="margin-left: 6px">{e.detail}</span>{/if}</span><span class="mono small faint">{open.right(e)}</span></div>
        {:else}
          {#if pv}<div class="frow"><span></span><span class="small faint">Nothing here.</span><span></span></div>{/if}
        {/each}
      </div>
      {#if unlisted}
        <span class="field-help" style="color: var(--warn)">{unlisted === 1 ? "1 mod isn't" : `${unlisted} mods aren't`} on Modrinth or CurseForge, so a mod list can't point to a download for {unlisted === 1 ? 'it' : 'them'}. A Modrinth or Full zip export brings {unlisted === 1 ? 'it' : 'them'} along.</span>
      {/if}
    {/if}
  </div>
  {#snippet foot()}
    <button class="btn" onclick={() => session.close()}>Cancel</button>
    <button class="btn btn-accent" onclick={run} disabled={busy || !inst.data || (isList && !preview.data?.listed.length)}>{code ? 'Done' : sharing ? 'Get code' : 'Export'}</button>
  {/snippet}
</Dialog>

<style>
  .stats{display:grid;overflow:hidden}
  .stat{display:flex;flex-direction:column;gap:2px;min-width:0;padding:10px 12px;border:0;border-left:1px solid var(--line);background:none;text-align:left;color:var(--text)}
  .stat:first-child{border-left:0}
  button.stat:hover{background:var(--raise)}
  .stat.on{background:var(--hover)}
  .stat .n{font-size:16px;line-height:22px;font-weight:600}
  .frow{display:grid;grid-template-columns:20px minmax(0,1fr) auto;align-items:center;column-gap:10px;height:36px;padding:0 12px;border-top:1px solid var(--line)}
  .frow:first-child{border-top:0}
  .frow :global(.ico){width:20px;height:20px;flex-basis:20px}
  .codebox{display:flex;align-items:center;gap:12px;padding:12px 14px;border:1px solid var(--line-2);border-radius:var(--radius-lg);background:var(--bg)}
  .code{font-family:var(--mono);font-size:22px;line-height:28px;letter-spacing:.24em;font-weight:500;font-variant-numeric:tabular-nums}
</style>

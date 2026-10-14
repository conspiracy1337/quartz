<script lang="ts">
  import { api } from '../api';
  import type { ExportFormat, ExportInclude, ExportListFormat, ExportMeta } from '../api/types';
  import { bytes, plural } from '../format';
  import { query } from '../query.svelte';
  import { session } from '../session.svelte';
  import { tasks } from '../tasks.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Seg from '../ui/Seg.svelte';
  import ToggleRow from '../ui/ToggleRow.svelte';

  /** Export an instance (composed: Dialog, .field rows, the shared Seg and a .tgl-grid of ToggleRows, as Create shortcut). */
  interface Props { instanceId: string }
  let { instanceId }: Props = $props();

  const FORMATS = [
    { value: 'mrpack', label: 'Modpack (.mrpack)' },
    { value: 'zip', label: 'Full zip' },
    { value: 'list', label: 'Mod list' },
  ] as const;
  const LISTS = [
    { value: 'md', label: 'Markdown' },
    { value: 'csv', label: 'CSV' },
  ] as const;
  const PARTS: { key: keyof ExportInclude; label: string }[] = [
    { key: 'options', label: 'Options and keybinds' },
    { key: 'servers', label: 'Server list' },
    { key: 'config', label: 'Mod settings' },
    { key: 'worlds', label: 'Worlds' },
    { key: 'screenshots', label: 'Screenshots' },
  ];

  const inst = query(() => api.instances.get(instanceId));
  let format = $state<ExportFormat>('mrpack');
  let list = $state<ExportListFormat>('md');
  let include = $state<ExportInclude>({ options: true, servers: true, config: true, worlds: false, screenshots: false });
  let busy = $state(false);
  /** The pack's name, version and summary (Prism's export fields); never the instance's private notes. */
  let meta = $state<ExportMeta>({ name: '', version: '' });
  // svelte-ignore state_referenced_locally
  void api.exports.meta(instanceId).then((m) => (meta = { summary: '', ...m }));

  const plan = $derived({ format, list, include: { ...include } });
  const preview = query(() => (plan.format === 'list' ? Promise.resolve(null) : api.exports.preview(instanceId, plan)));
  const help = $derived.by(() => {
    if (format === 'list') return 'Lists the mods that are turned on, with their versions and links.';
    const p = preview.data;
    if (!p) return '';
    if (format === 'zip') return `${plural(p.bundled, 'file')} bundled, ${bytes(p.bytes)}. Imports without downloading anything.`;
    return `${plural(p.linked, 'file')} ${p.linked === 1 ? 'downloads' : 'download'} from Modrinth; ${p.bundled} ${p.bundled === 1 ? 'is' : 'are'} bundled.`;
  });

  async function run() {
    if (busy) return;
    busy = true;
    try {
      const res = await api.exports.run(instanceId, { ...plan, meta: { ...meta } });
      if (!res.job && !res.file) return; // the save dialog was cancelled
      session.close();
      if (res.job) {
        await tasks.load();
        session.notify({ subject: `Exporting ${inst.data?.name ?? 'the instance'}`, text: res.file ?? '' });
      } else {
        session.notify({ subject: 'Mod list saved', text: res.file ?? '' });
      }
    } catch (e) {
      session.fail('Export', e);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog title="Export {inst.data?.name ?? 'instance'}" sub="Share it, or keep a copy." width={520} onclose={() => session.close()}>
  <div class="field"><span class="field-label">Format</span><Seg options={FORMATS} bind:value={format} label="Format" fill center /></div>
  {#if format === 'list'}
    <div class="field"><span class="field-label">File type</span><Seg options={LISTS} bind:value={list} label="File type" fill center /></div>
  {:else}
    <div style="display: grid; grid-template-columns: minmax(0,1fr) 120px; gap: 12px">
      <label class="field"><span class="field-label">Name</span><span class="input"><input bind:value={meta.name} maxlength="64" placeholder={inst.data?.name} /></span></label>
      <label class="field"><span class="field-label">Version</span><span class="input"><input bind:value={meta.version} maxlength="32" placeholder="1.0.0" /></span></label>
    </div>
    <label class="field"><span class="field-label">Summary</span><span class="input"><input bind:value={meta.summary} maxlength="500" placeholder="Optional" /></span></label>
    <div class="field"><span class="field-label">Include</span>
      <div class="card tgl-grid">
        {#each PARTS as part (part.key)}<ToggleRow name={part.label} on={include[part.key]} onchange={(v) => (include[part.key] = v)} />{/each}
      </div>
    </div>
  {/if}
  <span class="field-help">{help}</span>
  {#snippet foot()}
    <button class="btn" onclick={() => session.close()}>Cancel</button>
    <button class="btn btn-accent" onclick={run} disabled={busy || !inst.data}>Export</button>
  {/snippet}
</Dialog>

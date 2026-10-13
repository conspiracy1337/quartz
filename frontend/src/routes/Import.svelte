<script lang="ts">
  import { api } from '$lib/api';
  import type { ImportPreview, Src } from '$lib/api/types';
  import { hasBridge, onEvent } from '$lib/bridge';
  import { bytes, errorText, plural } from '$lib/format';
  import { artOf } from '$lib/look';
  import { go, href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import { tasks } from '$lib/tasks.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Tile from '$lib/ui/Tile.svelte';

  /** Opened with a pack from Browse, or empty to drop a file or paste a link. */
  interface Props { ref?: string | undefined; source?: Src | undefined; open?: string | undefined }
  let { ref, source, open }: Props = $props();

  // The preview shows the design's filled-in state.
  const START = hasBridge() ? '' : 'https://modrinth.com/modpack/cobblemon-fabric';
  let link = $state(START);
  let name = $state('');
  let pack = $state<ImportPreview | null>(null);
  let inspecting = $state(false);
  let busy = $state(false);
  let seq = 0;

  async function inspect(read: () => Promise<ImportPreview | null>) {
    const n = ++seq;
    inspecting = true;
    try {
      const p = await read();
      if (n !== seq) return;
      if (p) {
        pack = p;
        name = p.name;
      }
    } catch (e) {
      if (n === seq) session.notify({ tone: 'error', subject: "Couldn't read that pack", text: errorText(e) });
    } finally {
      if (n === seq) inspecting = false;
    }
  }

  // svelte-ignore state_referenced_locally
  if (open) void inspect(() => api.imports.inspectOpened(open!));
  else if (ref && source) void inspect(() => api.imports.inspectProject(ref!, source!));
  else if (!hasBridge()) void inspect(() => api.imports.inspectLink(link));

  // A pasted link is read once typing pauses.
  let lastLink = START;
  $effect(() => {
    const v = link.trim();
    if (v === lastLink || !v.startsWith('https://')) return;
    const t = setTimeout(() => {
      lastLink = v;
      void inspect(() => api.imports.inspectLink(v));
    }, 600);
    return () => clearTimeout(t);
  });
  $effect(() => onEvent<{ dropId: string }>('content:dropped', (d) => void inspect(() => api.imports.inspectDropped(d.dropId))));

  const art = $derived(pack ? (pack.art ?? artOf(pack.name, pack.icon)) : undefined);
  const loaderLine = $derived(pack ? `${pack.version ? `v${pack.version} for ` : 'For '}${pack.loader || 'Minecraft'} ${pack.minecraft}` : '');
  /** Overrides: "config, 212 files". */
  const overrides = $derived(pack?.stats.files ? `${pack.stats.folders.slice(0, 2).join(', ')}, ${plural(pack.stats.files, 'file')}` : 'None');
  // An imported instance is sandboxed where games can be (Go's import does the same).
  let sandboxed = $state(false);
  void api.sandbox.status().then((st) => (sandboxed = st.available));
  const warnings = $derived.by(() => {
    if (!pack) return [];
    const out: { title: string; detail: string }[] = [];
    const inLayers = pack.stats.foreign.filter((f) => f.includes('/'));
    const outside = pack.stats.foreign.filter((f) => !f.includes('/'));
    const list = (l: string[]) => `${l.slice(0, 2).join(', ')}${l.length > 2 ? ` and ${l.length - 2} more` : ''}.`;
    if (inLayers.length) out.push({ title: `${plural(inLayers.length, 'jar')} in overrides ${inLayers.length === 1 ? 'is' : 'are'} not from ${pack.source}`, detail: list(inLayers) + (sandboxed ? ` ${inLayers.length === 1 ? 'It' : 'They'} will run inside the sandbox.` : '') });
    if (outside.length) out.push({ title: `${plural(outside.length, 'jar')} ${outside.length === 1 ? 'downloads' : 'download'} from outside ${pack.source}`, detail: list(outside) });
    if (pack.stats.blocked) out.push({ title: `${plural(pack.stats.blocked, 'file')} ${pack.stats.blocked === 1 ? 'needs' : 'need'} a manual download`, detail: 'Their authors only allow downloads from the CurseForge website.' });
    for (const w of pack.warnings) out.push({ title: w, detail: '' });
    return out;
  });

  async function install() {
    if (!pack || !art) return;
    busy = true;
    try {
      // The tile is a tint and a letter; Go sets the pack's own icon when it has one.
      const job = await api.imports.install(pack.token, name.trim(), artOf(name.trim()));
      if (job) await tasks.load();
      go({ name: 'downloads' });
    } catch (e) {
      busy = false;
      session.notify({ tone: 'error', subject: `Couldn't install ${name.trim()}`, text: errorText(e) });
    }
  }
</script>

<div class="topbar">
  <a href={href({ name: 'library' })} class="btn btn-sm btn-ghost btn-icon" aria-label="Back"><Icon name="chevron-left" size={14} sw={2} /></a>
  <h1 class="h2" style="margin-right: auto">Import an instance</h1>
</div>
<div class="scroll">
  <div class="page nc">
    <section aria-label="Source" style="display: flex; flex-direction: column; gap: 14px">
      <div class="drop">
        <Icon name="upload" size={22} />
        <span>Drop a .mrpack, CurseForge zip or instance zip</span>
        <button class="btn btn-sm" onclick={() => void inspect(() => api.imports.inspectFile())}>Choose file</button>
      </div>
      <label class="field"><span class="field-label">Or paste a link</span><span class="input" style="height: 36px"><Icon name="link" size={14} sw={1.8} /><input bind:value={link} placeholder="https://" /></span><span class="field-help">Modrinth, CurseForge, FTB and Technic links work. So do direct file links.</span></label>
      <div class="small faint">Formats: Modrinth .mrpack, CurseForge zip, Prism and MultiMC zip, Technic, FTB.</div>
    </section>
    {#if pack && !inspecting}
      <section class="card" aria-label="Detected">
        <div style="display: grid; grid-template-columns: 56px minmax(0,1fr); gap: 12px; align-items: center; padding: 16px">
          <Tile art={art!} style="width: 56px; height: 56px; border-radius: 12px; font-size: 20px" />
          <div style="min-width: 0"><h2 class="h2 trunc">{pack.name}</h2><div class="small muted trunc">{loaderLine}</div></div>
        </div>
        <div class="divider"></div>
        <div class="card-pad" style="display: flex; flex-direction: column; gap: 12px">
          <div class="kv small" style="grid-template-columns: 96px minmax(0,1fr); row-gap: 8px"><span>Source</span><span>{pack.source}</span><span>Mods</span><span class="tnum">{pack.stats.mods}{pack.stats.modBytes ? `, ${bytes(pack.stats.modBytes)}` : ''}</span><span>Overrides</span><span class="tnum trunc">{overrides}</span></div>
          {#each warnings as w (w.title)}
            <div class="card" style="padding: 10px 12px; border-color: color-mix(in srgb, var(--warn) 35%, var(--line)); display: grid; grid-template-columns: 16px minmax(0,1fr); gap: 10px">
              <Icon name="warn" size={16} sw={1.8} color="var(--warn)" />
              <span class="small"><span style="display: block; font-weight: 500">{w.title}</span>{#if w.detail}<span class="faint" style="display: block">{w.detail}</span>{/if}</span>
            </div>
          {/each}
          <label class="field"><span class="field-label">Instance name</span><span class="input"><input bind:value={name} maxlength="64" /></span></label>
          <button class="btn btn-accent btn-block" disabled={!name.trim() || busy} onclick={install}>Install</button>
        </div>
      </section>
    {:else if inspecting}
      <section class="card" aria-label="Reading the pack" aria-busy="true">
        <div style="display: grid; grid-template-columns: 56px minmax(0,1fr); gap: 12px; align-items: center; padding: 16px">
          <div class="sk" style="width: 56px; height: 56px; border-radius: 12px"></div>
          <div style="display: flex; flex-direction: column; gap: 8px"><div class="sk" style="height: 16px; width: 70%"></div><div class="sk" style="height: 10px; width: 45%"></div></div>
        </div>
        <div class="divider"></div>
        <div class="card-pad" style="display: flex; flex-direction: column; gap: 10px"><div class="sk" style="height: 10px; width: 60%"></div><div class="sk" style="height: 10px; width: 50%"></div><div class="sk" style="height: 10px; width: 55%"></div><div class="sk" style="height: 36px; width: 100%"></div></div>
      </section>
    {/if}
  </div>
</div>

<style>
  .drop{height:150px;border:1px dashed var(--line-2);border-radius:12px;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:8px;color:var(--muted);background:var(--panel)}
  .nc{display:grid;grid-template-columns:minmax(0,1fr) 340px;gap:20px;align-items:start}
  @media (max-width:1100px){.nc{grid-template-columns:minmax(0,1fr)}}
</style>

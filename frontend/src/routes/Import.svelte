<script lang="ts">
  import { api } from '$lib/api';
  import type { ImportPreview, Look, Src } from '$lib/api/types';
  import LookEditor from '$lib/components/LookEditor.svelte';
  import { hasBridge, onEvent } from '$lib/bridge';
  import { bytes, errorText, plural } from '$lib/format';
  import { artOf, isPicture, isServed } from '$lib/look';
  import { go, href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import { tasks } from '$lib/tasks.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Tile from '$lib/ui/Tile.svelte';
  import ToggleRow from '$lib/ui/ToggleRow.svelte';

  /** Opened with a pack from Browse, or empty to drop a file or paste a link. */
  interface Props { ref?: string | undefined; source?: Src | undefined; version?: string | undefined; open?: string | undefined }
  let { ref, source, version, open }: Props = $props();
  // A pack from Browse or its project page gets the pack card alone, with what that page
  // already showed of it (name, picture, summary) drawn at once while Go reads the pack.
  // svelte-ignore state_referenced_locally
  const fromBrowse = !!(ref && source);
  // svelte-ignore state_referenced_locally
  const hint = session.packHint?.ref === ref ? session.packHint : null;
  session.packHint = null;

  // The preview shows the design's filled-in state.
  const START = hasBridge() ? '' : 'https://modrinth.com/modpack/cobblemon-fabric';
  let link = $state(START);
  let name = $state(hint?.name ?? '');
  let named = false; // typed by the player: the pack's own name no longer replaces it
  let look = $state<Look>({ art: { background: artOf(hint?.name ?? '').background, initial: '' }, icon: hint?.icon ?? '', banner: '' });
  const startIcon = hint?.icon ?? ''; // the picture until the player changes it
  let pack = $state<ImportPreview | null>(null);
  /** The optional files picked (Prism asks the same; all off by default). */
  let enable = $state<Record<string, boolean>>({});
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
        if (!named) name = p.name;
        if (look.icon === startIcon && p.icon) look.icon = p.icon;
        enable = {};
      }
    } catch (e) {
      if (n === seq) session.notify({ tone: 'error', subject: "Couldn't read that pack", text: errorText(e) });
    } finally {
      if (n === seq) inspecting = false;
    }
  }

  // svelte-ignore state_referenced_locally
  if (open) void inspect(() => api.imports.inspectOpened(open!));
  else if (ref && source) void inspect(() => api.imports.inspectProject(ref!, source!, version ?? ''));
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
      // From Browse the player's look goes in; a served picture is the pack's own icon, which
      // Go stores. Otherwise the tile is a tint and a letter, and Go sets the pack's icon.
      const l = fromBrowse
        ? { art: look.art, icon: isPicture(look.icon) ? look.icon : '', banner: look.banner, packIcon: isServed(look.icon) }
        : { art: artOf(name.trim()), icon: '', banner: '', packIcon: true };
      const job = await api.imports.install(pack.token, name.trim(), l, Object.keys(enable).filter((k) => enable[k]));
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
  <h1 class="h2" style="margin-right: auto">{fromBrowse ? 'Install modpack' : 'Import an instance'}</h1>
</div>

{#snippet facts(pack: ImportPreview)}
  <div class="kv small" style="grid-template-columns: 96px minmax(0,1fr); row-gap: 8px"><span>Source</span><span>{pack.source}</span><span>Mods</span><span class="tnum">{pack.stats.mods}{pack.stats.modBytes ? `, ${bytes(pack.stats.modBytes)}` : ''}</span><span>Overrides</span><span class="tnum trunc">{overrides}</span></div>
  {#if pack.existing}
    {@const ex = pack.existing}
    <div class="card" style="padding: 10px 12px; display: grid; grid-template-columns: 16px minmax(0,1fr) auto; gap: 10px; align-items: center">
      <Icon name="info" size={16} sw={1.8} color="var(--info)" />
      <span class="small"><span style="display: block; font-weight: 500">You have {ex.name}</span><span class="faint" style="display: block">Version {ex.version} is installed.</span></span>
      <button class="btn btn-sm" onclick={() => { session.packPick = { instanceId: ex.id, version: ex.target }; go({ name: 'instance', id: ex.id, tab: 'overview', sub: 'update' }); }}>Update it instead</button>
    </div>
  {/if}
  {#each warnings as w (w.title)}
    <div class="card" style="padding: 10px 12px; border-color: color-mix(in srgb, var(--warn) 35%, var(--line)); display: grid; grid-template-columns: 16px minmax(0,1fr); gap: 10px">
      <Icon name="warn" size={16} sw={1.8} color="var(--warn)" />
      <span class="small"><span style="display: block; font-weight: 500">{w.title}</span>{#if w.detail}<span class="faint" style="display: block">{w.detail}</span>{/if}</span>
    </div>
  {/each}
  {#if pack.optional?.length}
    <div class="field"><span class="field-label">Optional files</span>
      <div class="card tgl-grid one">
        {#each pack.optional as f (f.path)}<ToggleRow name={f.name} on={!!enable[f.path]} onchange={(v) => (enable[f.path] = v)} />{/each}
      </div>
    </div>
  {/if}
{/snippet}

<div class="scroll">
  {#if fromBrowse}
    <div class="one">
      <section class="card" aria-label="Modpack" aria-busy={!pack}>
        <div style="padding: 16px; min-width: 0">
          {#if hint || pack}
            <h2 class="h2 trunc">{pack?.name ?? hint?.name}</h2>
            <div class="small muted trunc">{pack ? loaderLine : `by ${hint?.author}`}</div>
            {#if hint?.summary}<p class="small faint clamp2" style="margin: 6px 0 0">{hint.summary}</p>{/if}
          {:else}
            <div style="display: flex; flex-direction: column; gap: 8px"><div class="sk" style="height: 16px; width: 50%"></div><div class="sk" style="height: 10px; width: 35%"></div></div>
          {/if}
        </div>
        <div class="divider"></div>
        <div class="card-pad" style="display: flex; flex-direction: column; gap: 14px">
          {#if pack && !inspecting}
            {@render facts(pack)}
          {:else}
            <div style="display: flex; flex-direction: column; gap: 10px" aria-hidden="true"><div class="sk" style="height: 10px; width: 40%"></div><div class="sk" style="height: 10px; width: 32%"></div><div class="sk" style="height: 10px; width: 36%"></div></div>
          {/if}
          <label class="field"><span class="field-label">Instance name</span><span class="input"><input bind:value={name} oninput={() => (named = true)} placeholder="Enter Instance Name" maxlength="64" /></span></label>
          <div class="field"><span class="field-label">Appearance</span><LookEditor bind:look {name} meta={pack ? `${pack.minecraft}${pack.loader ? `, ${pack.loader}` : ''}` : ''} loader={pack?.loader.toLowerCase() ?? ''} /></div>
          <button class="btn btn-accent btn-lg btn-block" disabled={!pack || inspecting || !name.trim() || busy} onclick={install}>{busy ? 'Downloading the pack…' : 'Install'}</button>
        </div>
      </section>
    </div>
  {:else}
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
          {@render facts(pack)}
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
  {/if}
</div>

<style>
  .one{max-width:760px;margin:0 auto;padding:28px 24px 32px}
  .drop{height:150px;border:1px dashed var(--line-2);border-radius:12px;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:8px;color:var(--muted);background:var(--panel)}
  .nc{display:grid;grid-template-columns:minmax(0,1fr) 340px;gap:20px;align-items:start}
  @media (max-width:1100px){.nc{grid-template-columns:minmax(0,1fr)}}
</style>

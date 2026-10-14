<script lang="ts">
  import { api } from '../api';
  import { ago } from '../format';
  import { SETTINGS_TAB_LABELS, instanceMeta } from '../labels';
  import { query } from '../query.svelte';
  import { go, router, type Route } from '../router.svelte';
  import { descText, registry } from '../settings/registry';
  import { marks, rank } from '../fuzzy';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';
  import type { IconName } from '../ui/icons';
  import Tile from '../ui/Tile.svelte';
  import { hasBridge } from '../bridge';

  /** Ctrl+K commands and Ctrl+P instance switcher; Tab flips between them. */
  interface Props { mode: 'commands' | 'instances' }
  let { mode }: Props = $props();

  const SHOWN = 5; // instance rows, as the design's switcher shows
  /** extra: more text the query may match (a setting's description and group). */
  interface Item { key: string; group: string; label: string; icon: IconName; hint?: string; extra?: string; run: () => void }
  const nav = (r: Route) => () => go(r);
  // Mod update actions: the design's sample instance in the preview, the most recent instance in the app.
  const recent = query(() => (hasBridge() ? api.instances.first() : Promise.resolve(undefined)));
  const pending = query(() => (recent.data ? api.instances.pendingUpdates(recent.data.id) : Promise.resolve([])));
  const updateItems = $derived.by((): Item[] => {
    if (!hasBridge()) {
      return [
        { key: 'check', group: 'Actions', label: 'Check for updates in all instances', icon: 'refresh', run: nav({ name: 'instance', id: 'fo', tab: 'mods', sub: 'updates' }) },
        { key: 'update', group: 'Actions', label: 'Update 2 mods in Fabulously Optimized', icon: 'arrow-up', run: nav({ name: 'instance', id: 'fo', tab: 'mods', sub: 'updates' }) },
      ];
    }
    const r = recent.data;
    if (!r) return [];
    const to = nav({ name: 'instance', id: r.id, tab: 'mods', sub: 'updates' });
    const n = pending.data?.length ?? 0;
    return [
      { key: 'check', group: 'Actions', label: 'Check for updates in all instances', icon: 'refresh', run: () => void checkAll() },
      ...(n ? [{ key: 'update', group: 'Actions', label: `Update ${n} ${n === 1 ? 'mod' : 'mods'} in ${r.name}`, icon: 'arrow-up' as const, run: to }] : []),
    ];
  });
  // Checks every instance's mods; the result toast opens the first one's review.
  async function checkAll() {
    session.notify({ tone: 'info', subject: 'Checking for updates', text: 'in every instance…' });
    try {
      const found = await api.content.checkAll();
      const total = found.reduce((n, f) => n + f.updates, 0);
      if (!found.length) return session.notify({ subject: 'Everything is up to date', text: '' });
      const first = found[0]!;
      session.notify({
        subject: `${total} ${total === 1 ? 'update' : 'updates'}`, text: found.length === 1 ? `in ${first.name}` : `in ${found.length} instances`,
        action: { label: 'Review', run: () => go({ name: 'instance', id: first.instanceId, tab: 'mods', sub: 'updates' }) },
      });
    } catch (e) {
      session.fail('Check for updates', e);
    }
  }
  const COMMANDS = $derived<Item[]>([
    ...updateItems,
    ...(session.update?.status === 'ready' ? [{ key: 'self', group: 'Actions', label: `Install launcher update ${session.update.version}`, icon: 'shield' as const, run: () => void api.updates.restart().catch((e) => session.fail('Restart', e)) }] : []),
    ...(session.update && session.update.status !== 'off' && session.update.status !== 'ready'
      ? [{ key: 'self-check', group: 'Actions', label: 'Check for launcher updates', icon: 'refresh' as const, run: () => void api.updates.check().then((u) => (session.update = u)).catch((e) => session.fail('Check for updates', e)) }]
      : []),
    ...(router.route.name === 'instance' ? [
      { key: 'shortcut', group: 'Actions', label: 'Create shortcut…', icon: 'link' as const, run: () => session.open({ kind: 'shortcut', instanceId: (router.route as { id: string }).id }) },
      { key: 'export', group: 'Actions', label: 'Export instance…', icon: 'upload' as const, run: () => session.open({ kind: 'export', instanceId: (router.route as { id: string }).id }) },
    ] : []),
    { key: 'import-launchers', group: 'Actions', label: 'Import from other launchers', icon: 'download', run: nav({ name: 'setup', step: 3 }) },
    { key: 'data-folder', group: 'Actions', label: 'Open data folder', icon: 'folder', run: () => void api.app.openDataFolder() },
    { key: 'new', group: 'Actions', label: 'Create a new instance', icon: 'plus', run: nav({ name: 'new-instance' }) },
    { key: 'shortcuts', group: 'Actions', label: 'Show keyboard shortcuts', icon: 'settings-simple', run: () => session.open({ kind: 'shortcuts' }) },
    ...registry.map((d) => ({ key: d.id, group: 'Settings', label: d.title, icon: 'settings-simple' as const, hint: `Settings, ${SETTINGS_TAB_LABELS[d.tab]}`, extra: `${descText(d)} ${d.group}`, run: nav({ name: 'settings', tab: d.tab }) })),
  ]);

  const instances = query(() => api.instances.list());
  let q = $state('');
  let sel = $state(0);
  let input: HTMLInputElement;
  $effect(() => input.focus());

  const needle = $derived(q.trim().toLowerCase());
  // Best matches first (see fuzzy.ts), then kept together by group so the arrow keys
  // walk the rows in the order they show.
  const ranked = $derived(rank(COMMANDS, needle, (c) => c.label, (c) => c.extra ?? '').slice(0, 8));
  const groups = $derived([...new Set(ranked.map((r) => r.item.group))]);
  const commands = $derived(groups.flatMap((g) => ranked.filter((r) => r.item.group === g)));
  const insts = $derived(rank(instances.data ?? [], needle, (i) => i.name, instanceMeta));
  const count = $derived(mode === 'commands' ? commands.length : Math.min(insts.length, SHOWN));

  // Read the row before closing: closing unmounts the palette and drops its props.
  function runAt(i: number, open = false) {
    const cmd = mode === 'commands' ? commands[i]?.item : undefined;
    const inst = mode === 'commands' ? undefined : insts[i]?.item;
    session.close();
    if (cmd) cmd.run();
    else if (inst) open ? go({ name: 'instance', id: inst.id, tab: 'overview' }) : session.play(inst.id);
  }
  function keys(e: KeyboardEvent) {
    if (e.key === 'Escape') session.close();
    else if (e.key === 'ArrowDown') sel = Math.min(count - 1, sel + 1);
    else if (e.key === 'ArrowUp') sel = Math.max(0, sel - 1);
    else if (e.key === 'Enter') runAt(sel, e.ctrlKey);
    else if (e.key === 'Tab') session.open({ kind: 'palette', mode: mode === 'commands' ? 'instances' : 'commands' });
    else if (e.key === '?' && !q) session.open({ kind: 'shortcuts' });
    else return;
    e.preventDefault();
  }
  $effect(() => {
    void needle;
    sel = 0;
  });
</script>

<div class="pal-scrim" role="presentation" onpointerdown={(e) => e.target === e.currentTarget && session.close()}>
  <div class="pal" class:narrow={mode === 'instances'} role="dialog" aria-label={mode === 'commands' ? 'Command palette' : 'Switch instance'}>
    <label class="pin">
      {#if mode === 'commands'}<Icon name="search" size={16} sw={1.8} style="color: var(--faint)" />{:else}<span class="badge">Instances</span>{/if}
      <span class="sr">{mode === 'commands' ? 'Command' : 'Find instance'}</span>
      <input bind:this={input} bind:value={q} onkeydown={keys} />
      {#if mode === 'commands'}<span class="kbd">Esc</span>{/if}
    </label>
    {#if mode === 'commands'}
      <div role="listbox" aria-label="Results">
        {#each groups as g (g)}
          <div class="grp-c"><div class="gl">{g}</div>
            {#each commands as { item: c, at }, i (c.key)}
              {#if c.group === g}
                <button class="pi" class:on={sel === i} role="option" aria-selected={sel === i} onpointermove={() => (sel = i)} onclick={() => runAt(i)}><Icon name={c.icon} /><span class="trunc">{#each marks(c.label, at) as m, k (k)}{#if m.hit}<mark>{m.text}</mark>{:else}{m.text}{/if}{/each}</span>{#if sel === i && !c.hint}<span class="k"><span class="kbd">Enter</span></span>{:else}<span class="small faint">{c.hint ?? ''}</span>{/if}</button>
              {/if}
            {/each}
          </div>
        {/each}
      </div>
      <div class="pfoot-p"><span><span class="kbd">↑</span> <span class="kbd">↓</span> move</span><span><span class="kbd">Enter</span> run</span><span><span class="kbd">Tab</span> switch to instances</span><button class="link" style="margin-left: auto" onclick={() => session.open({ kind: 'shortcuts' })}><span class="kbd">?</span> shortcuts</button></div>
    {:else}
      <div role="listbox" aria-label="Instances" style="padding: 8px">
        {#each insts.slice(0, SHOWN) as { item: r }, i (r.id)}
          <button class="pi pi-inst" class:on={sel === i} role="option" aria-selected={sel === i} onpointermove={() => (sel = i)} onclick={() => runAt(i)}>
            <Tile art={r.art} icon={r.icon} />
            <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{r.name}</span><span class="trunc small faint" style="display: block">{instanceMeta(r)}, {r.lastPlayed ? `played ${ago(r.lastPlayed)}` : 'never played'}</span></span>
            <span class="acts"><span class="kbd">Enter</span><span class="small faint">play</span><span class="kbd" style="margin-left: 6px">Ctrl Enter</span><span class="small faint">open</span></span>
          </button>
        {/each}
      </div>
      <div class="pfoot-p"><span><span class="kbd">↑</span> <span class="kbd">↓</span> move</span><span><span class="kbd">Tab</span> commands</span><span style="margin-left: auto">{Math.min(insts.length, SHOWN)} of {instances.data?.length ?? 0}</span></div>
    {/if}
  </div>
</div>

<style>
  /* The designs show the page behind at 40% (.ghost); a scrim in the page colour gives the same look. */
  .pal-scrim{position:absolute;inset:0;z-index:20;background:color-mix(in srgb,var(--bg) 60%,transparent)}
  .pal{position:absolute;left:50%;top:96px;transform:translateX(-50%);width:min(600px,calc(100% - 48px));border:1px solid var(--line-2);border-radius:12px;background:var(--raise);box-shadow:0 32px 80px -16px rgba(0,0,0,.85);z-index:10;overflow:hidden}
  .pal.narrow{width:min(560px,calc(100% - 48px))}
  .pin{display:flex;align-items:center;gap:10px;height:52px;padding:0 16px;border-bottom:1px solid var(--line)}
  .pin input{all:unset;flex:1;font-size:15px;color:var(--text)}
  .grp-c{padding:8px 8px 4px}
  .gl{height:24px;padding:0 8px;font-size:12px;color:var(--faint);display:flex;align-items:center}
  .pi{display:grid;grid-template-columns:20px minmax(0,1fr) auto;gap:12px;align-items:center;height:40px;padding:0 8px;border-radius:6px;border:0;background:transparent;width:100%;text-align:left;color:var(--text)}
  .pi.pi-inst{grid-template-columns:32px minmax(0,1fr) auto;height:52px;padding:0 10px;border-radius:8px}
  .pi.on{background:var(--hover)}
  .pi .k{display:flex;gap:4px}
  .acts{display:none;gap:4px;align-items:center}
  .pi.on .acts{display:flex}
  .pfoot-p{display:flex;gap:14px;align-items:center;height:36px;padding:0 14px;border-top:1px solid var(--line);font-size:12px;color:var(--faint)}
  .link{border:0;background:none;padding:0;color:inherit;font-size:12px}
  mark{background:none;color:var(--accent);font-weight:600}
</style>

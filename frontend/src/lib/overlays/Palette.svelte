<script lang="ts">
  import { api } from '../api';
  import { agoShort } from '../format';
  import { instanceMeta } from '../labels';
  import { query } from '../query.svelte';
  import { go, type Route } from '../router.svelte';
  import { registry } from '../settings/registry';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';
  import type { IconName } from '../ui/icons';
  import Tile from '../ui/Tile.svelte';
  import { hasBridge } from '../bridge';

  /** Ctrl+K commands and Ctrl+P instance switcher; Tab flips between them. */
  interface Props { mode: 'commands' | 'instances' }
  let { mode }: Props = $props();

  interface Item { key: string; group: string; label: string; icon: IconName; hint?: string; run: () => void }
  const nav = (r: Route) => () => go(r);
  const COMMANDS: Item[] = [
    // Mod update actions point at the design's sample instance until mod updates exist (M5).
    ...(hasBridge() ? [] : ([
      { key: 'check', group: 'Actions', label: 'Check for updates in all instances', icon: 'refresh', run: nav({ name: 'instance', id: 'fo', tab: 'mods', sub: 'updates' }) },
      { key: 'update', group: 'Actions', label: 'Update 2 mods in Fabulously Optimized', icon: 'arrow-up', run: nav({ name: 'instance', id: 'fo', tab: 'mods', sub: 'updates' }) },
    ] as Item[])),
    { key: 'self', group: 'Actions', label: 'Install launcher update 1.4.0', icon: 'shield', run: () => {} },
    { key: 'new', group: 'Actions', label: 'Create a new instance', icon: 'plus', run: nav({ name: 'new-instance' }) },
    { key: 'shortcuts', group: 'Actions', label: 'Show keyboard shortcuts', icon: 'settings-simple', run: () => session.open({ kind: 'shortcuts' }) },
    ...registry.map((d) => ({ key: d.id, group: 'Settings', label: d.title, icon: 'settings-simple' as const, hint: `Settings, ${d.group}`, run: nav({ name: 'settings', tab: d.tab }) })),
  ];

  const instances = query(() => api.instances.list());
  let q = $state('');
  let sel = $state(0);
  let input: HTMLInputElement;
  $effect(() => input.focus());

  const needle = $derived(q.trim().toLowerCase());
  const commands = $derived(COMMANDS.filter((c) => !needle || c.label.toLowerCase().includes(needle)).slice(0, 8));
  const insts = $derived((instances.data ?? []).filter((i) => !needle || `${i.name} ${instanceMeta(i)}`.toLowerCase().includes(needle)));
  const count = $derived(mode === 'commands' ? commands.length : Math.min(insts.length, 6));
  const groups = $derived([...new Set(commands.map((c) => c.group))]);

  function runAt(i: number, open = false) {
    session.close();
    if (mode === 'commands') commands[i]?.run();
    else if (insts[i]) open ? go({ name: 'instance', id: insts[i].id, tab: 'overview' }) : session.play(insts[i].id);
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
  /** Splits a label around the query for highlighting. */
  function hl(label: string): [string, string, string] {
    const at = needle ? label.toLowerCase().indexOf(needle) : -1;
    return at < 0 ? [label, '', ''] : [label.slice(0, at), label.slice(at, at + needle.length), label.slice(at + needle.length)];
  }
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
            {#each commands as c, i (c.key)}
              {#if c.group === g}
                {@const [a, m, b] = hl(c.label)}
                <button class="pi" class:on={sel === i} role="option" aria-selected={sel === i} onpointermove={() => (sel = i)} onclick={() => runAt(i)}><Icon name={c.icon} /><span class="trunc">{a}{#if m}<mark>{m}</mark>{/if}{b}</span>{#if sel === i && !c.hint}<span class="k"><span class="kbd">Enter</span></span>{:else}<span class="small faint">{c.hint ?? ''}</span>{/if}</button>
              {/if}
            {/each}
          </div>
        {/each}
      </div>
      <div class="pfoot-p"><span><span class="kbd">↑</span> <span class="kbd">↓</span> move</span><span><span class="kbd">Enter</span> run</span><span><span class="kbd">Tab</span> switch to instances</span><button class="link" style="margin-left: auto" onclick={() => session.open({ kind: 'shortcuts' })}><span class="kbd">?</span> shortcuts</button></div>
    {:else}
      <div role="listbox" aria-label="Instances" style="padding: 8px">
        {#each insts.slice(0, 6) as r, i (r.id)}
          <button class="pi pi-inst" class:on={sel === i} role="option" aria-selected={sel === i} onpointermove={() => (sel = i)} onclick={() => runAt(i)}>
            <Tile art={r.art} />
            <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{r.name}</span><span class="trunc small faint" style="display: block">{instanceMeta(r)}, {r.lastPlayed ? `played ${agoShort(r.lastPlayed).toLowerCase()}` : 'never played'}</span></span>
            <span class="acts"><span class="kbd">Enter</span><span class="small faint">play</span><span class="kbd" style="margin-left: 6px">Ctrl Enter</span><span class="small faint">open</span></span>
          </button>
        {/each}
      </div>
      <div class="pfoot-p"><span><span class="kbd">↑</span> <span class="kbd">↓</span> move</span><span><span class="kbd">Tab</span> commands</span><span style="margin-left: auto">{Math.min(insts.length, 6)} of {instances.data?.length ?? 0}</span></div>
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

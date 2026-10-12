<script lang="ts">
  import { MediaQuery } from 'svelte/reactivity';
  import { api } from '$lib/api';
  import type { Skin } from '$lib/api/types';
  import PlayerModel from '$lib/components/PlayerModel.svelte';
  import { searchIn } from '$lib/format';
  import { capeLook, capeTile, figure, skinLook } from '$lib/look';
  import { query } from '$lib/query.svelte';
  import { revs } from '$lib/revs.svelte';
  import { session } from '$lib/session.svelte';
  import SearchField from '$lib/ui/SearchField.svelte';
  import Empty from '$lib/ui/Empty.svelte';
  import PromptDialog from '$lib/ui/PromptDialog.svelte';
  import Seg from '$lib/ui/Seg.svelte';

  /** The wardrobe dresses the default account. */
  const accounts = query(() => api.accounts.list());
  const me = $derived(accounts.data?.find((a) => a.isDefault));
  const skins = query(() => api.accounts.skins());
  const capes = query(() => (me ? api.accounts.capes(me.id) : Promise.resolve({ capes: [], worn: '' })));
  let search = $state('');
  let skinSel = $state<string | null>(null);
  let capeSel = $state<string | null>(null);
  let busy = $state(false);
  let prompt = $state<'player' | 'link' | 'rename' | null>(null);
  // The model grows with the window height.
  const tall = new MediaQuery('min-height: 940px');
  const NO_CAPE = 'repeating-linear-gradient(45deg,var(--line-2) 0 4px,transparent 4px 8px)';
  const ARMS = [{ value: 'classic', label: 'Classic arms' }, { value: 'slim', label: 'Slim arms' }] as const;

  const all = $derived(skins.data ?? []);
  const list = $derived(all.filter((s) => s.name.toLowerCase().includes(search.trim().toLowerCase())));
  const cur = $derived(all.find((s) => s.hash === (skinSel ?? me?.skinHash)) ?? all[0]);
  const capeList = $derived(capes.data?.capes ?? []);
  const capeId = $derived(capeSel ?? capes.data?.worn ?? '');
  const cape = $derived(capeList.find((c) => c.id === capeId));
  const wearing = $derived(!!me && !!cur && me.skinHash === cur.hash && capeId === (capes.data?.worn ?? ''));

  /** Runs a library or account change, then reloads everything that shows it. */
  async function act<T>(what: string, fn: () => Promise<T>): Promise<T | undefined> {
    busy = true;
    try {
      const r = await fn();
      revs.bump('accounts');
      return r;
    } catch (e) {
      session.fail(what, e);
      return undefined;
    } finally {
      busy = false;
    }
  }

  async function added(p: Promise<Skin>) {
    const s = await p;
    revs.bump('accounts');
    if (s.hash) skinSel = s.hash;
  }
  const upload = () => act('Upload skin', () => added(api.accounts.importSkinFile()));

  async function wear() {
    if (!me || !cur) return;
    const ok = await act('Wear skin', () => api.accounts.wear(me.id, cur.hash, capeId));
    if (ok) session.notify({ tone: 'info', subject: me.name, text: `now wears ${cur.name}` });
  }

  async function remove(s: Skin) {
    const gone = await act('Remove skin', () => api.accounts.removeSkin(s.hash));
    if (!gone) return;
    skinSel = null;
    session.notify({ tone: 'trash', subject: s.name, text: 'from your skins', undo: () => void act('Restore skin', () => api.accounts.restoreSkin(gone)) });
  }

  const setArms = (v: 'classic' | 'slim') => cur && act('Arm type', () => api.accounts.setSkinSlim(cur.hash, v === 'slim'));
  const resetSkin = () => me && act('Default skin', () => api.accounts.resetSkin(me.id));
</script>

<div class="wd">
  <section class="scroll" aria-label="Wardrobe">
    <div class="page" style="gap: 20px">
      <div class="wd-tools">
        <h1 class="h1">Skins</h1>
        <SearchField bind:value={search} label="Search skins" placeholder={searchIn('skins', skins.data?.length)} />
        <button class="btn" onclick={() => (prompt = 'player')}>Copy from player</button>
        <button class="btn" onclick={() => (prompt = 'link')}>From link</button>
        <button class="btn btn-accent" disabled={busy} onclick={upload}>Upload skin</button>
      </div>
      <section aria-label="Your skins" style="display: flex; flex-direction: column; gap: 10px">
        <h2 class="h3">Your skins <span class="count">{skins.data?.length ?? ''}</span></h2>
        <div class="wg" role="radiogroup" aria-label="Skin">
          {#each list as s (s.hash)}
            {@const f = figure(s)}
            <button class="wc" class:on={cur?.hash === s.hash} onclick={() => (skinSel = s.hash)} role="radio" aria-checked={cur?.hash === s.hash}>
              <span style="display: grid; place-items: center"><span class="fig" aria-hidden="true" style="grid-template-columns: {f.arm}px 8px {f.arm}px; width: {8 + 2 * f.arm}px"><span class="h" style="background: {f.h}"></span><span class="al" style="background: {f.al}"></span><span class="b" style="background: {f.b}"></span><span class="ar" style="background: {f.ar}"></span><span class="l"><span style="background: {f.ll}"></span><span style="background: {f.lr}"></span></span></span></span>
              <span class="nm"><span class="trunc">{s.name}</span>{#if me?.skinHash === s.hash}<span class="badge badge-accent">Wearing</span>{/if}</span>
            </button>
          {/each}
          {#if skins.data && !list.length}<Empty text={all.length ? 'No skins match your search.' : 'No skins yet. Upload one, or copy one from a player or a link.'} style="grid-column: 1 / -1" />{/if}
          {#if !skins.data}{#each { length: 8 } as _, k (k)}<div class="sk" style="height: 180px; border-radius: var(--radius-lg)" aria-hidden="true"></div>{/each}{/if}
        </div>
      </section>
      <section aria-label="Capes" style="display: flex; flex-direction: column; gap: 10px">
        <h2 class="h3">Cape <span class="count">{capes.data ? capeList.length : ''}</span></h2>
        <div class="wg" role="radiogroup" aria-label="Cape">
          {#if capes.data}
            <button class="cp" class:on={capeId === ''} onclick={() => (capeSel = '')} role="radio" aria-checked={capeId === ''}><span class="cp-art" style="background: {NO_CAPE}" aria-hidden="true"></span><span class="trunc small">No cape</span></button>
            {#each capeList as c (c.id)}
              <button class="cp" class:on={capeId === c.id} onclick={() => (capeSel = c.id)} role="radio" aria-checked={capeId === c.id}><span class="cp-art" style="background: {capeTile(c, 30, 46)}" aria-hidden="true"></span><span class="trunc small">{c.name}</span></button>
            {/each}
          {:else}
            {#each { length: 3 } as _, k (k)}<div class="sk" style="height: 96px; border-radius: var(--radius-lg)" aria-hidden="true"></div>{/each}
          {/if}
        </div>
      </section>
    </div>
  </section>
  <aside class="wd-prev" aria-label="Selected skin">
    <div class="wd-model">
      {#if cur}<PlayerModel {...skinLook(cur)} {...capeLook(cape)} fill scale={tall.current ? 11 : 8} />{/if}
    </div>
    <div class="wd-info">
      <div style="min-width: 0"><div class="h2 trunc">{cur?.name ?? ''}</div><div class="small faint">{cape ? `${cape.name} cape` : 'No cape'}, {cur?.slim ? 'slim' : 'classic'} arms</div></div>
      <Seg options={ARMS} value={cur?.slim ? 'slim' : 'classic'} label="Arm type" fill onchange={setArms} />
      <button class="btn btn-accent btn-lg btn-block" disabled={!me || !cur || busy || wearing} onclick={wear}>{wearing ? 'Wearing this skin and cape' : 'Wear this skin and cape'}</button>
      <div style="display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 8px">
        <button class="btn btn-sm" disabled={!cur} onclick={() => (prompt = 'rename')}>Rename</button>
        <button class="btn btn-sm" disabled={!me?.skinHash || busy} onclick={resetSkin}>Default skin</button>
        <button class="btn btn-sm btn-danger" disabled={!cur || busy} onclick={() => cur && remove(cur)}>Remove</button>
      </div>
    </div>
  </aside>
</div>

{#if prompt === 'player'}
  <PromptDialog title="Copy from player" label="Player name" help="Copies the skin that player wears right now." action="Copy skin" maxlength={16}
    valid={(v) => /^[A-Za-z0-9_]{1,16}$/.test(v.trim())} submit={(v) => added(api.accounts.importSkinPlayer(v))} onclose={() => (prompt = null)} />
{:else if prompt === 'link'}
  <PromptDialog title="Skin from link" label="Link to a skin PNG" help="A 64×64 or 64×32 PNG, starting with https://." action="Add skin" placeholder="https://"
    valid={(v) => v.trim().startsWith('https://')} submit={(v) => added(api.accounts.importSkinURL(v))} onclose={() => (prompt = null)} />
{:else if prompt === 'rename' && cur}
  {@const target = cur}
  <PromptDialog title="Rename skin" label="Name" help="1 to 40 characters." action="Rename" value={target.name} maxlength={40}
    submit={(v) => api.accounts.renameSkin(target.hash, v).then(() => revs.bump('accounts'))} onclose={() => (prompt = null)} />
{/if}

<style>
  /* Two panes that fill the window: the wardrobe scrolls on the left, the preview takes the full height on the right
     and widens with the window (300px at the default size, up to 460px when maximised). */
  .wd{flex:1 1 auto;min-height:0;display:grid;grid-template-columns:minmax(0,1fr) clamp(300px,30%,460px)}
  .wd-tools{display:flex;flex-wrap:wrap;align-items:center;gap:8px}
  .wd-tools .h1{margin-right:8px}
  .wd-tools :global(.input){flex:1 1 200px}
  /* Skins and capes share one column grid, so cape tiles line up under skin tiles at every width. */
  .wg{display:grid;grid-template-columns:repeat(auto-fill,minmax(140px,1fr));gap:var(--gap)}
  .wc{height:180px;border-radius:var(--radius-lg);border:1px solid var(--line);background:var(--panel);display:grid;grid-template-rows:1fr 36px;padding:0;color:var(--text);overflow:hidden}
  .wc:hover{border-color:var(--line-2)}
  .wc.on{border-color:var(--accent);box-shadow:0 0 0 1px var(--accent)}
  .wc .nm{display:flex;align-items:center;justify-content:space-between;gap:6px;padding:0 12px;border-top:1px solid var(--line);font-size:12px}
  .fig{display:grid;grid-template-columns:4px 8px 4px;grid-template-rows:8px 12px 12px;width:16px;transform:scale(3.4);image-rendering:pixelated;margin:auto}
  .fig .h{grid-column:2;grid-row:1}.fig .b{grid-column:2;grid-row:2}.fig .al{grid-column:1;grid-row:2}.fig .ar{grid-column:3;grid-row:2}.fig .l{grid-column:2;grid-row:3;display:grid;grid-template-columns:1fr 1fr}
  .cp{height:96px;border-radius:var(--radius-lg);border:1px solid var(--line);background:var(--panel);display:flex;flex-direction:column;align-items:center;justify-content:center;gap:8px;padding:0 12px;color:var(--text)}
  .cp:hover{border-color:var(--line-2)}
  .cp.on{border-color:var(--accent);box-shadow:0 0 0 1px var(--accent)}
  .cp-art{width:30px;height:46px;border-radius:3px;box-shadow:inset 0 0 0 1px rgba(255,255,255,.06);image-rendering:pixelated}
  .wd-prev{display:flex;flex-direction:column;min-height:0;min-width:0;border-left:1px solid var(--line)}
  .wd-model{position:relative;flex:1 1 auto;min-height:0;background:radial-gradient(60% 50% at 50% 52%,#17171b 0%,var(--bg) 75%)}
  .wd-info{display:flex;flex-direction:column;gap:12px;padding:20px var(--gutter) var(--gutter);border-top:1px solid var(--line);background:var(--panel)}
</style>

<script lang="ts">
  import { MediaQuery } from 'svelte/reactivity';
  import { api } from '$lib/api';
  import PlayerModel from '$lib/components/PlayerModel.svelte';
  import { searchIn } from '$lib/format';
  import { query } from '$lib/query.svelte';
  import SearchField from '$lib/ui/SearchField.svelte';

  const skins = query(() => api.accounts.skins());
  const capes = query(() => api.accounts.capes());
  let search = $state('');
  let skinId = $state<string | null>(null);
  let capeId = $state<string | null>(null);
  // The model grows with the window height.
  const tall = new MediaQuery('min-height: 940px');

  const list = $derived((skins.data ?? []).filter((s) => s.name.toLowerCase().includes(search.trim().toLowerCase())));
  const cur = $derived(skins.data?.find((s) => s.id === (skinId ?? skins.data?.[1]?.id)));
  const capeList = $derived(capes.data?.capes ?? []);
  const capeSel = $derived(capeId ?? capes.data?.worn ?? 'none');
  const cape = $derived(capeList.find((c) => c.id === capeSel));
</script>

<div class="wd">
  <section class="scroll" aria-label="Wardrobe">
    <div class="page" style="gap: 20px">
      <div class="wd-tools">
        <h1 class="h1">Skins</h1>
        <SearchField bind:value={search} label="Search skins" placeholder={searchIn('skins', skins.data?.length)} />
        <button class="btn">Copy from player</button>
        <button class="btn">From link</button>
        <button class="btn btn-accent">Upload skin</button>
      </div>
      <section aria-label="Your skins" style="display: flex; flex-direction: column; gap: 10px">
        <h2 class="h3">Your skins <span class="count">{skins.data?.length ?? ''}</span></h2>
        <div class="wg" role="radiogroup" aria-label="Skin">
          {#each list as s (s.id)}
            {@const c = s.colors}
            <button class="wc" class:on={cur?.id === s.id} onclick={() => (skinId = s.id)} role="radio" aria-checked={cur?.id === s.id}>
              <span style="display: grid; place-items: center"><span class="fig" aria-hidden="true"><span class="h" style="background: linear-gradient({c.hair} 0 30%, {c.skin} 30%)"></span><span class="al" style="background: linear-gradient({c.top} 0 33%, {c.skin} 33%)"></span><span class="b" style="background: {c.top}"></span><span class="ar" style="background: linear-gradient({c.top} 0 33%, {c.skin} 33%)"></span><span class="l"><span style="background: linear-gradient({c.pants} 0 75%, #1b1d22 75%)"></span><span style="background: linear-gradient({c.pants} 0 75%, #1b1d22 75%)"></span></span></span></span>
              <span class="nm"><span class="trunc">{s.name}</span>{#if s.worn}<span class="badge badge-accent">Wearing</span>{/if}</span>
            </button>
          {/each}
          {#if !skins.data}{#each { length: 8 } as _, k (k)}<div class="sk" style="height: 180px; border-radius: var(--radius-lg)" aria-hidden="true"></div>{/each}{/if}
        </div>
      </section>
      <section aria-label="Capes" style="display: flex; flex-direction: column; gap: 10px">
        <h2 class="h3">Cape <span class="count">{capeList.length ? capeList.length - 1 : ''}</span></h2>
        <div class="wg" role="radiogroup" aria-label="Cape">
          {#each capeList as c (c.id)}
            <button class="cp" class:on={capeSel === c.id} onclick={() => (capeId = c.id)} role="radio" aria-checked={capeSel === c.id}><span style="width: 30px; height: 46px; border-radius: 3px; background: {c.art}; box-shadow: inset 0 0 0 1px rgba(255,255,255,.06)" aria-hidden="true"></span><span class="trunc small">{c.name}</span></button>
          {/each}
        </div>
      </section>
    </div>
  </section>
  <aside class="wd-prev" aria-label="Selected skin">
    <div class="wd-model">
      {#if cur}<PlayerModel colors={cur.colors} cape={cape && cape.id !== 'none' ? cape.art : undefined} slim={cur.slim} fill scale={tall.current ? 11 : 8} />{/if}
    </div>
    <div class="wd-info">
      <div style="min-width: 0"><div class="h2 trunc">{cur?.name ?? ''}</div><div class="small faint">{cape && cape.id !== 'none' ? `${cape.name} cape` : 'No cape'}, {cur?.slim ? 'slim' : 'classic'} arms</div></div>
      <button class="btn btn-accent btn-lg btn-block">Wear this skin and cape</button>
      <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 8px"><button class="btn btn-sm">Rename</button><button class="btn btn-sm btn-danger">Remove</button></div>
    </div>
  </aside>
</div>

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
  .wd-prev{display:flex;flex-direction:column;min-height:0;min-width:0;border-left:1px solid var(--line)}
  .wd-model{position:relative;flex:1 1 auto;min-height:0;background:radial-gradient(60% 50% at 50% 52%,#17171b 0%,var(--bg) 75%)}
  .wd-info{display:flex;flex-direction:column;gap:12px;padding:20px var(--gutter) var(--gutter);border-top:1px solid var(--line);background:var(--panel)}
</style>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import { api } from '../api';
  import type { Look } from '../api/types';
  import { errorText } from '../format';
  import { TINTS } from '../look';
  import Icon from '../ui/Icon.svelte';
  import Swatches from '../ui/Swatches.svelte';
  import InstanceCard from './InstanceCard.svelte';

  /** An instance's look: colour, letter, picture and banner, beside the Library card they
      make as a live preview. Composed from the Appearance swatch row, the instance card and
      input-styled picker fields. Colour, picture and banner change independently: a new
      colour never drops a picture. Used by New instance, a modpack's install card and the
      Appearance dialog. `look.icon` may also be a served picture (a modpack's icon). */
  /** `loader` ("" for vanilla) is whose mark the tile shows without a picture or letter.
      `children` is an optional leading field above the colour (New instance's name). */
  interface Props { look: Look; name: string; meta: string; loader: string; left?: string; right?: string; children?: Snippet }
  let { look = $bindable(), name, meta, loader, left = 'Not played yet', right = '0 h', children }: Props = $props();

  let error = $state('');

  async function pick(kind: 'icon' | 'banner') {
    error = '';
    try {
      const hash = await (kind === 'icon' ? api.instances.chooseIcon() : api.instances.chooseBanner());
      if (hash) look = { ...look, [kind]: hash };
    } catch (e) {
      error = errorText(e);
    }
  }
</script>

{#snippet picture(label: string, kind: 'icon' | 'banner')}
  <div class="field"><span class="field-label">{label}</span>
    <span class="input pick">
      <button class="pick-main" onclick={() => pick(kind)}><Icon name="upload" size={12} sw={2} />{look[kind] ? 'Change' : 'Choose'}</button>
      {#if look[kind]}<button class="btn-bare" aria-label="Remove {label.toLowerCase()}" onclick={() => (look = { ...look, [kind]: '' })}><Icon name="x" size={10} sw={2.2} /></button>{/if}
    </span>
  </div>
{/snippet}

<div class="look">
  <InstanceCard art={look.art} icon={look.icon} {loader} banner={look.banner} name={name.trim() || 'Instance name'} {meta} {left} {right} />
  <div style="display: flex; flex-direction: column; gap: 12px; min-width: 0">
    {@render children?.()}
    <div class="field"><span class="field-label">Colour</span><Swatches colors={TINTS} value={look.art.background} label="Tile colour" onpick={(c) => (look = { ...look, art: { ...look.art, background: c } })} /></div>
    <div class="look-row">
      {@render picture('Picture', 'icon')}
      {@render picture('Banner', 'banner')}
      <label class="field"><span class="field-label">Letter</span><span class="input"><input value={look.art.initial ?? ''} placeholder="None" maxlength="2" oninput={(e) => (look = { ...look, art: { ...look.art, initial: e.currentTarget.value.trim() } })} /></span></label>
    </div>
    {#if error}<span class="field-help" style="color: var(--warn)">{error}</span>{/if}
  </div>
</div>

<style>
  .look{display:grid;grid-template-columns:220px minmax(0,1fr);gap:var(--gap);align-items:start}
  .look-row{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr) 72px;gap:16px}
  .pick{padding-right:2px;transition:background-color 120ms ease}
  .pick:hover{background:var(--raise)}
  .pick-main{all:unset;flex:1;min-width:0;height:100%;display:flex;align-items:center;gap:8px;color:var(--text);cursor:pointer}
  .pick-main :global(svg){color:var(--faint)}
  @media (max-width:640px){.look{grid-template-columns:minmax(0,1fr)}}
</style>

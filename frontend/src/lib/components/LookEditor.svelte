<script lang="ts">
  import { api } from '../api';
  import type { Look } from '../api/types';
  import { errorText } from '../format';
  import { TINTS } from '../look';
  import Swatches from '../ui/Swatches.svelte';
  import InstanceCard from './InstanceCard.svelte';

  /** An instance's look: colour, letter, picture and banner, beside the Library card they
      make as a live preview. Composed from the Appearance swatch row, the instance card and
      the small button pairs. Colour, picture and banner change independently: a new colour
      never drops a picture. Used by New instance, a modpack's install card and the
      Appearance dialog. `look.icon` may also be a served picture (a modpack's icon). */
  interface Props { look: Look; name: string; meta: string; left?: string; right?: string }
  let { look = $bindable(), name, meta, left = 'Not played yet', right = '0 h' }: Props = $props();

  let error = $state('');
  const initial = $derived(look.art.initial || (/[\p{L}\p{N}]/u.exec(name)?.[0] ?? '?').toUpperCase());

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
    <span style="display: flex; gap: 6px"><button class="btn btn-sm" onclick={() => pick(kind)}>Change</button><button class="btn btn-sm btn-ghost" disabled={!look[kind]} onclick={() => (look = { ...look, [kind]: '' })}>Remove</button></span>
  </div>
{/snippet}

<div class="look">
  <InstanceCard art={{ background: look.art.background, initial }} icon={look.icon} banner={look.banner} name={name.trim() || 'Instance name'} {meta} {left} {right} />
  <div style="display: flex; flex-direction: column; gap: 12px; min-width: 0">
    <div class="field"><span class="field-label">Colour</span><Swatches colors={TINTS} value={look.art.background} label="Tile colour" onpick={(c) => (look = { ...look, art: { ...look.art, background: c } })} /></div>
    <div class="look-row">
      {@render picture('Picture', 'icon')}
      {@render picture('Banner', 'banner')}
      <label class="field"><span class="field-label">Letter</span><span class="input"><input value={look.art.initial ?? ''} placeholder={initial} maxlength="2" oninput={(e) => (look = { ...look, art: { ...look.art, initial: e.currentTarget.value.trim() } })} /></span></label>
    </div>
    <span class="field-help" style:color={error ? 'var(--warn)' : undefined}>{error || 'A PNG or JPEG. The picture is cropped to a square; the banner fills the top of the card.'}</span>
  </div>
</div>

<style>
  .look{display:grid;grid-template-columns:220px minmax(0,1fr);gap:var(--gap);align-items:start}
  .look-row{display:grid;grid-template-columns:auto auto 72px;gap:16px;justify-content:start}
  @media (max-width:640px){.look{grid-template-columns:minmax(0,1fr)}}
</style>

<script lang="ts">
  import type { Art } from '../api/types';
  import { errorText } from '../format';
  import { TINTS } from '../look';
  import Dialog from '../ui/Dialog.svelte';
  import InstanceMark from '../ui/InstanceMark.svelte';
  import Swatches from '../ui/Swatches.svelte';

  /**
   * Picks an instance's icon: a tile colour and letter, or a picture. Composed from the
   * existing dialog, the Appearance swatch row and the add-account field pattern.
   * `choose` opens the native picture picker and resolves with the picture's name.
   */
  interface Props {
    art: Art;
    icon?: string | undefined;
    choose: () => Promise<string>;
    save: (art: Art, icon: string) => Promise<unknown>;
    onclose: () => void;
  }
  let { art, icon, choose, save, onclose }: Props = $props();

  // svelte-ignore state_referenced_locally
  let color = $state(art.background);
  // svelte-ignore state_referenced_locally
  let letter = $state(art.initial ?? '');
  // svelte-ignore state_referenced_locally
  let picture = $state(icon && /^[0-9a-f]{64}$/.test(icon) ? icon : '');
  let busy = $state(false);
  let error = $state('');
  const preview = $derived<Art>({ background: color, initial: letter.trim() || art.initial || '' });

  async function pick() {
    error = '';
    try {
      picture = (await choose()) || picture;
    } catch (e) {
      error = errorText(e);
    }
  }
  async function done() {
    busy = true;
    error = '';
    try {
      await save(preview, picture);
      onclose();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog title="Change icon" width={480} {onclose}>
  <div style="display: grid; grid-template-columns: 56px minmax(0,1fr); gap: 16px; align-items: start">
    <span class="ico" style="width: 56px; height: 56px; border-radius: 12px; background: {preview.background}; font-size: 22px" aria-hidden="true"><InstanceMark art={preview} icon={picture} /></span>
    <div style="display: flex; flex-direction: column; gap: 12px; min-width: 0">
      <div class="field"><span class="field-label">Colour</span><Swatches colors={TINTS} value={color} label="Tile colour" onpick={(c) => { color = c; picture = ''; }} /></div>
      <label class="field"><span class="field-label">Letter</span><span class="input" style="width: 110px"><input bind:value={letter} maxlength="2" oninput={() => (picture = '')} /></span></label>
      <div class="field">
        <span class="field-label">Picture</span>
        <span style="display: flex; gap: 6px"><button class="btn btn-sm" onclick={pick}>Choose picture</button>{#if picture}<button class="btn btn-sm btn-ghost" onclick={() => (picture = '')}>Use the colour</button>{/if}</span>
        <span class="field-help" style:color={error ? 'var(--warn)' : undefined}>{error || 'A PNG or JPEG, cropped to a square.'}</span>
      </div>
    </div>
  </div>
  {#snippet foot()}
    <button class="btn" onclick={onclose}>Cancel</button>
    <button class="btn btn-accent" disabled={busy} onclick={done}>Save</button>
  {/snippet}
</Dialog>

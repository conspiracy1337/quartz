<script lang="ts">
  import { session } from '../session.svelte';
  import type { Control } from './registry';

  /** A write-only field: the stored value is never shown, only whether one exists. */
  interface Props { control: Extract<Control, { kind: 'secret' }>; label: string }
  let { control, label }: Props = $props();
  let has = $state(false);
  let draft = $state('');
  let busy = $state(false);

  $effect(() => {
    control.has().then((v) => (has = v)).catch(() => (has = false));
  });

  async function save() {
    busy = true;
    try {
      await control.save(draft);
      has = draft !== '';
      draft = '';
    } catch (e) {
      session.notify({ subject: label, text: e instanceof Error ? e.message : String(e) });
    } finally {
      busy = false;
    }
  }
</script>

<span style="display: grid; grid-template-columns: 1fr auto; gap: 8px">
  <label class="input"><span class="sr">{label}</span><input type="password" autocomplete="off" placeholder={has ? 'Saved' : 'Not set'} bind:value={draft} /></label>
  <button class="btn" disabled={busy || (!draft && !has)} onclick={save}>{draft || !has ? 'Save' : 'Remove'}</button>
</span>

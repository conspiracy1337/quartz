<script lang="ts">
  import { errorText } from '../format';
  import Dialog from './Dialog.svelte';

  /**
   * A one-field dialog, built like the add-account offline form: a field with help text,
   * then Cancel and the action. `submit` may throw; its message shows under the field.
   */
  interface Props {
    title: string;
    label: string;
    help: string;
    action: string;
    value?: string;
    placeholder?: string;
    maxlength?: number;
    valid?: (v: string) => boolean;
    submit: (v: string) => Promise<unknown>;
    onclose: () => void;
  }
  let { title, label, help, action, value = '', placeholder = '', maxlength = 200, valid = (v) => v.trim() !== '', submit, onclose }: Props = $props();
  // svelte-ignore state_referenced_locally
  let text = $state(value);
  let busy = $state(false);
  let error = $state('');

  async function go(e: SubmitEvent) {
    e.preventDefault();
    if (!valid(text) || busy) return;
    busy = true;
    error = '';
    try {
      await submit(text.trim());
      onclose();
    } catch (err) {
      error = errorText(err);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog {title} width={480} {onclose}>
  <form id="prompt" onsubmit={go} style="display: flex; flex-direction: column; gap: 12px">
    <!-- svelte-ignore a11y_autofocus -->
    <label class="field"><span class="field-label">{label}</span><span class="input"><input bind:value={text} {maxlength} {placeholder} autofocus /></span><span class="field-help" style:color={error ? 'var(--warn)' : undefined}>{error || help}</span></label>
  </form>
  {#snippet foot()}
    <button class="btn" onclick={onclose}>Cancel</button>
    <button class="btn btn-accent" type="submit" form="prompt" disabled={busy || !valid(text)}>{action}</button>
  {/snippet}
</Dialog>

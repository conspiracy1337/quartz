<script lang="ts">
  import { api } from '../api';
  import type { HostProperties } from '../api/host';
  import { query } from '../query.svelte';
  import { session } from '../session.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Icon from '../ui/Icon.svelte';

  /** The server's server.properties in the mono text field (DlgHostProps.dc.html). Text is
      dim; a line is bright when it differs from Minecraft's default or when the Host dialog
      sets it. A layer behind the see-through field draws it, so editing stays plain text. */
  interface Props { instanceId: string; name: string; onclose: () => void }
  let { instanceId, name, onclose }: Props = $props();

  const file = query(() => api.host.properties(instanceId));
  let text = $state<string | null>(null);
  $effect(() => {
    if (file.data && text === null) text = file.data.text;
  });
  let busy = $state(false);

  /** A line's key, as java.util.Properties reads it ("" for comments and blanks). */
  const keyOf = (line: string) => {
    const t = line.trimStart();
    return !t || t[0] === '#' || t[0] === '!' ? '' : (/^[^=:\s]+/.exec(t)?.[0] ?? '');
  };
  const bright = (p: HostProperties, line: string) => {
    const k = keyOf(line);
    if (!k) return false;
    const v = line.trimStart().slice(k.length).replace(/^\s*[=:]?\s*/, '');
    return p.dialog.includes(k) || p.defaults[k] !== v;
  };

  let back = $state<HTMLElement>();
  const sync = (e: Event) => {
    const t = e.currentTarget as HTMLTextAreaElement;
    if (back) [back.scrollTop, back.scrollLeft] = [t.scrollTop, t.scrollLeft];
  };

  async function save() {
    if (text === null) return;
    busy = true;
    try {
      await api.host.saveProperties(instanceId, text);
      onclose();
    } catch (e) {
      session.fail('server.properties', e);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog title="server.properties" sub="{name}'s server settings" width={620} {onclose}>
  {#if file.data && text !== null}
    {@const p = file.data}
    <label class="field hl"><span class="sr">server.properties</span>
      <pre class="hl-back" aria-hidden="true" bind:this={back}>{#each text.split('\n') as line, i (i)}{#if i > 0}{'\n'}{/if}{#if bright(p, line)}<b>{line}</b>{:else}{line}{/if}{/each}{'\n'}</pre>
      <textarea class="textarea" spellcheck="false" bind:value={text} onscroll={sync}></textarea>
    </label>
  {:else}
    <div class="sk" style="height: 380px; border-radius: var(--radius)" aria-hidden="true"></div>
  {/if}
  {#snippet foot()}
    <button class="btn btn-ghost" style="margin-right: auto" onclick={() => api.host.openFolder(instanceId).catch((e) => session.fail('Open folder', e))}><Icon name="folder" size={15} sw={1.8} />Open folder</button>
    <button class="btn" onclick={onclose}>Cancel</button>
    <button class="btn btn-accent" disabled={busy || text === null} onclick={save}>Save</button>
  {/snippet}
</Dialog>

<style>
  .hl{position:relative;border-radius:var(--radius);background:var(--panel)}
  .hl-back{position:absolute;inset:0;margin:0;padding:8px 10px;border:1px solid transparent;font-family:var(--mono);font-size:12px;line-height:18px;color:var(--faint);white-space:pre;overflow:hidden;pointer-events:none}
  .hl-back b{color:var(--text);font-weight:400}
  .hl .textarea{position:relative;display:block;height:380px;white-space:pre;resize:none;overflow:auto;background:transparent;color:transparent;caret-color:var(--text)}
</style>

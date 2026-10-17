<script lang="ts">
  import { api } from '../api';
  import { query } from '../query.svelte';
  import { revs } from '../revs.svelte';
  import { session } from '../session.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Tile from '../ui/Tile.svelte';

  /** Copy a world to another instance (DlgCopyWorld.dc.html): radio rows like Host's world list. */
  interface Props { instanceId: string; folder: string; name: string; sub: string }
  let { instanceId, folder, name, sub }: Props = $props();

  const instances = query(() => api.instances.list(), 'instances.list');
  const others = $derived((instances.data ?? []).filter((i) => i.id !== instanceId));
  const here = $derived(instances.data?.find((i) => i.id === instanceId)?.name ?? '');
  let target = $state('');
  $effect(() => {
    if (!target && others[0]) target = others[0].id;
  });
  let busy = $state(false);
  const close = () => session.close();

  async function copy() {
    if (!target || busy) return;
    busy = true;
    try {
      await api.worlds.copyTo(instanceId, folder, target);
      revs.bump('worlds');
      close();
      session.notify({ subject: name, text: `was copied to ${others.find((i) => i.id === target)?.name ?? 'the instance'}` });
    } catch (e) {
      session.fail('Copy world', e);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog title="Copy {name}" {sub} width={520} onclose={close}>
  <div class="field"><span class="field-label">To instance</span>
    <div class="list cw-list" role="radiogroup" aria-label="To instance">
      {#each others as i (i.id)}
        <button class="wr" class:on={target === i.id} role="radio" aria-checked={target === i.id} onclick={() => (target = i.id)}>
          <span class="radio" class:on={target === i.id} aria-hidden="true"></span>
          <Tile art={i.art} icon={i.icon} style="width: 32px; height: 32px; border-radius: 6px" />
          <span class="trunc">{i.name}</span>
          <span class="small faint">{i.minecraft}</span>
        </button>
      {/each}
    </div>
    <span class="field-help">The copy keeps its name. {name} stays in {here}.</span>
  </div>
  {#snippet foot()}
    <button class="btn" onclick={close}>Cancel</button>
    <button class="btn btn-accent" onclick={copy} disabled={busy || !target}>Copy world</button>
  {/snippet}
</Dialog>

<style>
  /* Five rows show; more instances scroll inside the list, so the dialog never outgrows the window. */
  .cw-list{max-height:240px;overflow-y:auto}
  .wr{display:grid;grid-template-columns:16px 32px minmax(0,1fr) auto;gap:12px;align-items:center;height:48px;padding:0 12px;border:0;background:transparent;width:100%;text-align:left;color:var(--text);border-top:1px solid var(--line)}
  .wr:first-child{border-top:0}
  .wr.on{background:var(--raise)}
</style>

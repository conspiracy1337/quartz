<script lang="ts">
  import { api } from '../api';
  import type { Look } from '../api/types';
  import LookEditor from '../components/LookEditor.svelte';
  import { agoShort, hours } from '../format';
  import { instanceMeta } from '../labels';
  import { query } from '../query.svelte';
  import { revs } from '../revs.svelte';
  import { session } from '../session.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import PromptDialog from '../ui/PromptDialog.svelte';

  /** Rename and Appearance, opened from either instance menu. */
  interface Props { kind: 'rename' | 'look'; instanceId: string }
  let { kind, instanceId }: Props = $props();

  const inst = query(() => api.instances.get(instanceId));
  const close = () => session.close();
  // The editor starts from the instance once it has loaded.
  let look = $state<Look | null>(null);
  $effect(() => {
    const i = inst.data;
    if (i && !look) look = { art: { ...i.art }, icon: i.icon ?? '', banner: i.banner ?? '' };
  });
  let busy = $state(false);
  async function save(l: Look) {
    busy = true;
    try {
      await api.instances.setLook(instanceId, l);
      revs.bump('instances');
      close();
    } catch (e) {
      session.fail('Appearance', e);
    } finally {
      busy = false;
    }
  }
</script>

{#if inst.data}
  {@const i = inst.data}
  {#if kind === 'rename'}
    <PromptDialog title="Rename {i.name}" label="Name" help="Its folder on your computer is renamed to match." action="Rename" value={i.name} maxlength={64}
      submit={async (v) => { await api.instances.rename(i.id, v); revs.bump('instances'); }} onclose={close} />
  {:else if look}
    <Dialog title="Appearance" width={640} onclose={close}>
      <LookEditor bind:look name={i.name} meta={instanceMeta(i)} loader={i.loader?.kind ?? ''} left={agoShort(i.lastPlayed)} right={hours(i.playtimeMin)} />
      {#snippet foot()}
        <button class="btn" onclick={close}>Cancel</button>
        <button class="btn btn-accent" disabled={busy} onclick={() => look && save(look)}>Save</button>
      {/snippet}
    </Dialog>
  {/if}
{/if}

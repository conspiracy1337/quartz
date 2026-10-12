<script lang="ts">
  import { api } from '../api';
  import type { Art } from '../api/types';
  import { query } from '../query.svelte';
  import { revs } from '../revs.svelte';
  import { session } from '../session.svelte';
  import PromptDialog from '../ui/PromptDialog.svelte';
  import IconDialog from './IconDialog.svelte';

  /** Rename and Change icon, opened from either instance menu. */
  interface Props { kind: 'rename' | 'icon'; instanceId: string }
  let { kind, instanceId }: Props = $props();

  const inst = query(() => api.instances.get(instanceId));
  const close = () => session.close();
  async function saveIcon(art: Art, icon: string) {
    await api.instances.setArt(instanceId, art);
    if (icon) await api.instances.setIcon(instanceId, icon);
    revs.bump('instances');
  }
</script>

{#if inst.data}
  {@const i = inst.data}
  {#if kind === 'rename'}
    <PromptDialog title="Rename {i.name}" label="Name" help="Only the name changes; the instance's folder stays where it is." action="Rename" value={i.name} maxlength={64}
      submit={async (v) => { await api.instances.rename(i.id, v); revs.bump('instances'); }} onclose={close} />
  {:else}
    <IconDialog art={i.art} icon={i.icon} choose={api.instances.chooseIcon} save={saveIcon} onclose={close} />
  {/if}
{/if}

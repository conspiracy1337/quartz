<script lang="ts">
  import { api } from '../api';
  import { revs } from '../revs.svelte';
  import { session } from '../session.svelte';
  import PromptDialog from '../ui/PromptDialog.svelte';

  /** Rename a world, built like Rename instance: only the name the game shows changes. */
  interface Props { instanceId: string; folder: string; name: string }
  let { instanceId, folder, name }: Props = $props();
</script>

<PromptDialog title="Rename {name}" label="Name" help="Only the name the game shows changes; the world's folder stays." action="Rename" value={name} maxlength={64}
  submit={async (v) => { await api.worlds.rename(instanceId, folder, v); revs.bump('worlds'); }} onclose={() => session.close()} />

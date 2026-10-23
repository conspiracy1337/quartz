<script lang="ts">
  import { api } from '../api';
  import { session } from '../session.svelte';
  import Dialog from '../ui/Dialog.svelte';

  /**
   * Why the Windows sandbox needs one administrator approval, shown before Windows asks
   * (its UAC prompt). Continue asks; a game then starts again, a server is started again
   * from its panel.
   */
  interface Props { instanceId: string; then: 'play' | 'host'; accountId?: string | undefined }
  let { instanceId, then, accountId = '' }: Props = $props();
  let busy = $state(false);

  async function approve() {
    if (busy) return;
    busy = true;
    try {
      await api.sandbox.approveListing();
      session.close();
      if (then === 'play') session.play(instanceId, accountId);
      else session.notify({ subject: 'Approved', text: 'start the server again' });
    } catch (e) {
      busy = false;
      session.fail('Approve', e);
    }
  }
</script>

<Dialog title="Allow the sandbox once" sub="Windows asks one time for the drive Quartz is on." width={520} onclose={() => session.close()}>
  <p class="muted" style="margin: 0">Minecraft and its mod loaders look up the folders above the game by name when they start. The sandbox hides even those names, so the game can't start until Windows allows it.</p>
  <p class="muted" style="margin: 0">This lets sandboxed games see the names of the folders above the Quartz folder and of your other instances. What's inside them, like files, worlds and accounts, stays out of reach. Windows asks for an administrator because some of those folders, like the drive itself, belong to the system.</p>
  <div class="card" style="padding: 10px 12px"><span class="small muted">This is no extra risk: without the sandbox, a mod can see all of this and read everything inside too.</span></div>
  {#snippet foot()}
    <button class="btn" onclick={() => session.close()}>Cancel</button>
    <button class="btn btn-accent" disabled={busy} onclick={approve}>Continue</button>
  {/snippet}
</Dialog>

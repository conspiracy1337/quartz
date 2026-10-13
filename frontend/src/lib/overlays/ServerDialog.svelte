<script lang="ts">
  import { api } from '../api';
  import type { ServerEntry } from '../api/types';
  import { count, errorText } from '../format';
  import { revs } from '../revs.svelte';
  import { session } from '../session.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Seg from '../ui/Seg.svelte';

  /** Add server, or Edit {name} (DlgServer.dc.html). The address is pinged as it is typed. */
  interface Props { instanceId: string; entry?: ServerEntry | undefined }
  let { instanceId, entry }: Props = $props();

  // svelte-ignore state_referenced_locally
  let name = $state(entry?.name ?? '');
  // svelte-ignore state_referenced_locally
  let address = $state(entry?.address ?? '');
  // svelte-ignore state_referenced_locally
  let packs = $state<ServerEntry['packs']>(entry?.packs ?? 'prompt');
  let busy = $state(false);
  let error = $state('');
  type Ping = { state: 'idle' | 'checking' | 'none' } | { state: 'online'; text: string };
  let ping = $state<Ping>({ state: 'idle' });

  const PACKS = [{ value: 'prompt', label: 'Ask' }, { value: 'enabled', label: 'Always use' }, { value: 'disabled', label: 'Never use' }] as const;
  const close = () => session.close();
  const valid = $derived(name.trim() !== '' && address.trim() !== '');

  $effect(() => {
    const a = address.trim();
    if (!a) {
      ping = { state: 'idle' };
      return;
    }
    ping = { state: 'checking' };
    let stale = false;
    const t = setTimeout(() => {
      api.servers.check(instanceId, a).then((s) => {
        if (stale) return;
        const players = s.max ? `${count(s.players)} of ${count(s.max)}` : count(s.players);
        ping = s.online ? { state: 'online', text: `Online, ${s.version ?? ''}, ${players} players, ${s.pingMs ?? 0} ms` } : { state: 'none' };
      }, () => !stale && (ping = { state: 'none' }));
    }, 500);
    return () => {
      stale = true;
      clearTimeout(t);
    };
  });

  async function save(e: SubmitEvent) {
    e.preventDefault();
    if (!valid || busy) return;
    busy = true;
    error = '';
    const next = { name: name.trim(), address: address.trim(), packs };
    try {
      if (entry) await api.servers.edit(instanceId, entry.index, entry.address, next);
      else await api.servers.add(instanceId, next);
      revs.bump('servers');
      close();
    } catch (err) {
      error = errorText(err);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog title={entry ? `Edit ${entry.name}` : 'Add server'} width={480} onclose={close}>
  <form id="server" onsubmit={save} style="display: flex; flex-direction: column; gap: 12px">
    <!-- svelte-ignore a11y_autofocus -->
    <label class="field"><span class="field-label">Name</span><span class="input"><input bind:value={name} maxlength={64} style="color: var(--text)" autofocus /></span></label>
    <label class="field"><span class="field-label">Address</span><span class="input"><input class="mono" bind:value={address} maxlength={255} spellcheck="false" style="color: var(--text)" /></span>
      {#if error}<span class="field-help" style="color: var(--warn)">{error}</span>
      {:else if ping.state === 'online'}<span class="field-help sv-ping"><span class="sv-dot" style="background: var(--accent)" aria-hidden="true"></span>{ping.text}</span>
      {:else if ping.state === 'checking'}<span class="field-help sv-ping"><span class="sv-dot" style="background: var(--line-2)" aria-hidden="true"></span>Checking the server</span>
      {:else if ping.state === 'none'}<span class="field-help sv-ping"><span class="sv-dot" style="background: var(--danger)" aria-hidden="true"></span>No reply from this address right now</span>
      {:else}<span class="field-help">Port 25565 unless you add one, like play.example.net:25570.</span>{/if}
    </label>
    <div class="field"><span class="field-label">Server resource packs</span>
      <Seg options={PACKS} bind:value={packs} label="Server resource packs" fill center />
      <span class="field-help">What happens when the server sends a pack.</span>
    </div>
  </form>
  {#snippet foot()}
    <button class="btn" onclick={close}>Cancel</button>
    <button class="btn btn-accent" type="submit" form="server" disabled={busy || !valid}>{entry ? 'Save' : 'Add server'}</button>
  {/snippet}
</Dialog>

<style>
  .sv-ping{display:flex;align-items:center;gap:6px}
  .sv-dot{width:8px;height:8px;border-radius:50%;flex:0 0 8px}
</style>

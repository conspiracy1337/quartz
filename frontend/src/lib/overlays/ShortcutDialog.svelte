<script lang="ts">
  import { api } from '../api';
  import { query } from '../query.svelte';
  import { session } from '../session.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Seg from '../ui/Seg.svelte';
  import Select from '../ui/Select.svelte';

  /** Create shortcut (composed: Dialog, .field rows, the shared Seg and Select, as Host's dialog). */
  interface Props { instanceId: string }
  let { instanceId }: Props = $props();

  type Place = 'desktop' | 'menu' | 'both';
  const PLACES = [
    { value: 'desktop', label: 'Desktop' },
    { value: 'menu', label: 'App menu' },
    { value: 'both', label: 'Both' },
  ] as const;

  const inst = query(() => api.instances.get(instanceId));
  const accounts = query(() => api.accounts.list(), 'accounts');
  const worlds = query(() => api.worlds.list(instanceId));
  const servers = query(() => api.servers.list(instanceId));

  let place = $state<Place>('desktop');
  let account = $state('');
  let into = $state('');
  let busy = $state(false);

  const accountOptions = $derived([{ value: '', label: 'Default account' }, ...(accounts.data ?? []).map((a) => ({ value: a.id, label: a.name }))]);
  // Values are "world:<folder>" or "server:<row id>"; the address is looked up on Create.
  const intoOptions = $derived([
    { value: '', label: 'Title screen' },
    ...(worlds.data ?? []).map((w) => ({ value: `world:${w.id}`, label: w.name })),
    ...(servers.data ?? []).map((s) => ({ value: `server:${s.id}`, label: s.name })),
  ]);
  const where = { desktop: 'on the desktop', menu: 'in the app menu', both: 'on the desktop and in the app menu' } as const;

  async function create() {
    if (busy) return;
    const [kind = '', ref = ''] = into ? [into.slice(0, into.indexOf(':')), into.slice(into.indexOf(':') + 1)] : [];
    const target = kind === 'server' ? (api.servers.entry(instanceId, ref)?.address ?? '') : ref;
    busy = true;
    try {
      await api.instances.createShortcut(instanceId, place, account, kind, target);
      session.close();
      session.notify({ subject: 'Shortcut created', text: where[place] });
    } catch (e) {
      session.fail('Create shortcut', e);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog title="Create shortcut" sub={inst.data ? `Starts ${inst.data.name} with one click.` : undefined} width={480} onclose={() => session.close()}>
  <div class="field"><span class="field-label">Where</span><Seg options={PLACES} bind:value={place} label="Where" fill center /></div>
  <div class="field"><span class="field-label">Account</span><Select options={accountOptions} bind:value={account} label="Account" /></div>
  <div class="field"><span class="field-label">Open into</span><Select options={intoOptions} bind:value={into} label="Open into" />
    <span class="field-help">The game opens straight into this world or server.</span></div>
  {#snippet foot()}
    <button class="btn" onclick={() => session.close()}>Cancel</button>
    <button class="btn btn-accent" onclick={create} disabled={busy || !inst.data}>Create</button>
  {/snippet}
</Dialog>

<script lang="ts">
  import { api } from '../api';
  import { revs } from '../revs.svelte';
  import { session, type AddAccountMode } from '../session.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Seg from '../ui/Seg.svelte';

  interface Props { mode?: AddAccountMode | undefined }
  let { mode: initial }: Props = $props();
  // The dialog opens on the requested method; later changes are the user's own.
  // svelte-ignore state_referenced_locally
  let mode = $state<AddAccountMode>(initial ?? 'code');
  let name = $state('');
  let saving = $state(false);
  const MODES = [{ value: 'browser', label: 'Browser' }, { value: 'code', label: 'Code' }, { value: 'offline', label: 'Offline' }] as const;
  const NAME = /^[A-Za-z0-9_]{3,16}$/;

  async function addOffline() {
    saving = true;
    try {
      await api.accounts.addOffline(name);
      revs.bump('accounts');
      session.close();
    } catch (e) {
      session.fail('Offline account', e);
    } finally {
      saving = false;
    }
  }

  // Placeholder QR pattern until M3 renders the real device-code QR: three finder squares plus noise.
  const cells = (() => {
    const finder = (x: number, y: number) => {
      for (const [a, b] of [[0, 0], [14, 0], [0, 14]] as const) {
        const dx = x - a, dy = y - b;
        if (dx >= 0 && dy >= 0 && dx <= 6 && dy <= 6) return dx === 0 || dy === 0 || dx === 6 || dy === 6 || (dx >= 2 && dx <= 4 && dy >= 2 && dy <= 4);
      }
      return null;
    };
    const out: boolean[] = [];
    let seed = 7;
    for (let y = 0; y < 21; y++) for (let x = 0; x < 21; x++) {
      seed = (seed * 1103515245 + 12345) % 2147483648;
      out.push(finder(x, y) ?? ((seed >> 16) & 1) === 1);
    }
    return out;
  })();
</script>

<Dialog title="Add an account" width={480} onclose={() => session.close()}>
  <Seg options={MODES} bind:value={mode} label="Sign-in method" fill />
  {#if mode === 'browser'}
    <div style="display: flex; flex-direction: column; gap: 12px; align-items: flex-start">
      <p class="muted" style="margin: 0">Sign in with Microsoft in your browser. You will come back here automatically.</p>
      <button class="btn btn-accent">Open Microsoft sign-in</button>
      <span class="small faint">Waiting for the browser…</span>
    </div>
  {:else if mode === 'code'}
    <div style="display: grid; grid-template-columns: 147px minmax(0,1fr); gap: 18px; align-items: center">
      <div class="qr" role="img" aria-label="QR code for microsoft.com/link">{#each cells as on, k (k)}<i class:on></i>{/each}</div>
      <div style="display: flex; flex-direction: column; gap: 10px; min-width: 0">
        <span class="small muted">Scan the code, or go to <a href="https://www.microsoft.com/link">microsoft.com/link</a> and enter:</span>
        <span class="code">K7QX-2M9P</span>
        <button class="btn btn-sm" style="align-self: flex-start">Copy code</button>
        <span class="small faint tnum">Expires in 14:32</span>
      </div>
    </div>
  {:else}
    <div style="display: flex; flex-direction: column; gap: 12px">
      <label class="field"><span class="field-label">Player name</span><span class="input"><input bind:value={name} maxlength="16" placeholder="TestDummy" /></span><span class="field-help">3 to 16 letters, numbers or underscores.</span></label>
      <div class="card" style="padding: 10px 12px"><span class="small muted">Offline accounts can play singleplayer and LAN, and servers that allow it. Skins only show locally.</span></div>
    </div>
  {/if}
  {#snippet foot()}
    <button class="btn" onclick={() => session.close()}>Cancel</button>
    {#if mode === 'offline'}<button class="btn btn-accent" disabled={saving || !NAME.test(name)} onclick={addOffline}>Add offline account</button>{/if}
  {/snippet}
</Dialog>

<style>
  .qr{display:grid;grid-template-columns:repeat(21,1fr);width:147px;height:147px;padding:10px;background:#f4f4f5;border-radius:8px}
  .qr i{display:block}
  .qr i.on{background:#0a0a0b}
  .code{font-family:var(--mono);font-size:26px;letter-spacing:.18em;font-weight:500}
</style>

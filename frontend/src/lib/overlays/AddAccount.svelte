<script lang="ts">
  import { copyText } from '../clipboard';
  import { onDestroy, untrack } from 'svelte';
  import { api } from '../api';
  import type { SignInEvent } from '../api/types';
  import { hasBridge, onEvent } from '../bridge';
  import { previewSignIn } from '../api/mock/accounts';
  import { query } from '../query.svelte';
  import { revs } from '../revs.svelte';
  import { session, type AddAccountMode } from '../session.svelte';
  import { countdown, idle, reduce } from '../signin';
  import Dialog from '../ui/Dialog.svelte';
  import Seg from '../ui/Seg.svelte';

  /** accountId asks to sign that account in again. */
  interface Props { mode?: AddAccountMode | undefined; accountId?: string | undefined }
  let { mode: initial, accountId }: Props = $props();
  // The dialog opens on the requested method; later changes are the user's own.
  // svelte-ignore state_referenced_locally
  let mode = $state<AddAccountMode>(initial ?? 'browser');
  let name = $state('');
  let saving = $state(false);
  let st = $state(idle());
  let started = $state(false);
  let now = $state(Date.now());
  const MODES = [{ value: 'browser', label: 'Browser' }, { value: 'code', label: 'Code' }, { value: 'offline', label: 'Offline' }] as const;
  const NAME = /^[A-Za-z0-9_]{3,16}$/;

  const ready = query(() => api.accounts.microsoftReady());
  const accounts = query(() => api.accounts.list(), 'accounts');
  const again = $derived(accountId ? accounts.data?.find((a) => a.id === accountId) : undefined);

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

  /** Starts the Microsoft sign-in of the current tab (a new one replaces the old). */
  async function start() {
    st = idle(st.flow);
    started = true;
    try {
      await (mode === 'browser' ? api.accounts.signInBrowser() : api.accounts.signInCode());
      if (!hasBridge()) st = reduce(st, previewSignIn(mode));
    } catch (e) {
      started = false;
      session.fail('Microsoft sign-in', e);
    }
  }

  async function finished(id: string) {
    revs.bump('accounts');
    const list = await api.accounts.list();
    session.close();
    session.notify({ tone: 'info', subject: list.find((a) => a.id === id)?.name ?? 'Account', text: 'is signed in' });
  }

  const off = onEvent<SignInEvent>('accounts:signin', (e) => {
    st = reduce(st, e);
    if (e.done && e.accountId && e.flow === st.flow) void finished(e.accountId);
  });

  // The Code tab gets its code as soon as it opens; leaving a tab ends its sign-in.
  $effect(() => {
    const m = mode;
    if (!ready.data) return;
    if (m === 'code') untrack(() => void start());
    return () => {
      started = false;
      void api.accounts.cancelSignIn();
    };
  });

  $effect(() => {
    if (mode !== 'code' || !st.expiresAt) return;
    const t = setInterval(() => (now = Date.now()), 1000);
    return () => clearInterval(t);
  });

  onDestroy(() => {
    off();
    void api.accounts.cancelSignIn();
  });

  const copy = () => void copyText(st.code);
  // Shown in two halves for reading aloud and typing ("K7QX-2M9P"); Copy keeps the raw code.
  const shown = $derived(st.code && !st.code.includes('-') ? `${st.code.slice(0, Math.ceil(st.code.length / 2))}-${st.code.slice(Math.ceil(st.code.length / 2))}` : st.code);
  const waiting = $derived(started && !st.error && !st.accountId);
</script>

<Dialog title="Add an account" sub={again ? `Sign in again as ${again.name}` : undefined} width={480} onclose={() => session.close()}>
  <Seg options={MODES} bind:value={mode} label="Sign-in method" fill />
  {#if mode !== 'offline' && ready.data === false}
    <div class="card" style="padding: 10px 12px"><span class="small muted">Microsoft sign-in isn't set up in this build. You can still add an offline account.</span></div>
  {:else if mode === 'browser'}
    <div style="display: flex; flex-direction: column; gap: 12px; align-items: flex-start">
      <p class="muted" style="margin: 0">Sign in with Microsoft in your browser. You will come back here automatically.</p>
      <button class="btn btn-accent" style="align-self: center" onclick={start}>Open Microsoft sign-in</button>
      {#if waiting}<span class="small faint" style="align-self: center">{st.step ? `${st.step}…` : 'Waiting for the browser…'}</span>{/if}
    </div>
  {:else if mode === 'code'}
    <div style="display: grid; grid-template-columns: 147px minmax(0,1fr); gap: 18px; align-items: center">
      {#if st.qr.length}
        <div class="qr" role="img" aria-label="QR code for microsoft.com/link" style="grid-template-columns: repeat({st.qr.length}, 1fr)">{#each st.qr.flat() as on, k (k)}<i class:on></i>{/each}</div>
      {:else}
        <div class="qr sk" aria-hidden="true"></div>
      {/if}
      <div style="display: flex; flex-direction: column; gap: 10px; min-width: 0">
        <span class="small muted">Scan the code, or go to <a href="https://www.microsoft.com/link" onclick={(e) => { e.preventDefault(); void api.app.openUrl('https://www.microsoft.com/link'); }}>microsoft.com/link</a> and enter:</span>
        {#if st.code}<span class="code">{shown}</span>{:else}<span class="code sk" aria-hidden="true">&nbsp;</span>{/if}
        <button class="btn btn-sm" style="align-self: flex-start" disabled={!st.code} onclick={copy}>Copy code</button>
        <span class="small faint tnum">{st.step ? `${st.step}…` : st.expiresAt ? `Expires in ${countdown(st.expiresAt, now)}` : '\u00a0'}</span>
      </div>
    </div>
  {:else}
    <div style="display: flex; flex-direction: column; gap: 12px">
      <label class="field"><span class="field-label">Player name</span><span class="input"><input bind:value={name} maxlength="16" placeholder="TestDummy" /></span><span class="field-help">3 to 16 letters, numbers or underscores.</span></label>
      <div class="card" style="padding: 10px 12px"><span class="small muted">Offline accounts can play singleplayer and LAN, and servers that allow it. Skins only show locally.</span></div>
    </div>
  {/if}
  {#if mode !== 'offline' && st.error && st.error.reason !== 'cancelled'}
    <div class="card sg-err" role="alert">
      <span class="small">{st.error.message}</span>
      <span style="display: flex; gap: 6px">
        {#if st.error.fixUrl}<button class="btn btn-sm" onclick={() => api.app.openUrl(st.error?.fixUrl ?? '')}>Open help page</button>{/if}
        <button class="btn btn-sm" onclick={start}>Try again</button>
      </span>
    </div>
  {/if}
  {#snippet foot()}
    <button class="btn" onclick={() => session.close()}>Cancel</button>
    {#if mode === 'offline'}<button class="btn btn-accent" disabled={saving || !NAME.test(name)} onclick={addOffline}>Add offline account</button>{/if}
  {/snippet}
</Dialog>

<style>
  .qr{display:grid;grid-template-columns:repeat(21,1fr);width:147px;height:147px;padding:10px;background:#f4f4f5;border-radius:8px}
  .qr.sk{background:var(--line)}
  .qr i{display:block}
  .qr i.on{background:#0a0a0b}
  .code{font-family:var(--mono);font-size:26px;letter-spacing:.18em;font-weight:500}
  .code.sk{width:12ch;border-radius:4px}
  /* A failed sign-in: the offline note's card with the alert bar's warn border. */
  .sg-err{display:grid;grid-template-columns:minmax(0,1fr) auto;align-items:center;gap:10px;padding:10px 12px;border-color:color-mix(in srgb,var(--warn) 30%,var(--line))}
</style>

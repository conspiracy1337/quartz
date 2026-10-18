<script lang="ts">
  import { api } from '$lib/api';
  import { revs } from '$lib/revs.svelte';
  import type { Account } from '$lib/api/types';
  import { query, skeletonRows } from '$lib/query.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';
  import Empty from '$lib/ui/Empty.svelte';
  import Menu, { below } from '$lib/ui/Menu.svelte';
  import MenuConfirm from '$lib/ui/MenuConfirm.svelte';
  import { since } from '$lib/format';
  import { avatar } from '$lib/look';
  import { prefs } from '$lib/prefs.svelte';

  const accounts = query(() => api.accounts.list(), 'accounts');
  const acc = $derived(prefs.value?.accounts);

  /** The row's "More" menu; removing asks once more inside it. */
  let more = $state<{ account: Account; x: number; y: number; confirm: boolean } | null>(null);
  function openMore(a: Account, e: MouseEvent) {
    more = { account: a, ...below(e), confirm: false };
  }
  async function makeDefault(a: Account) {
    more = null;
    try {
      await api.accounts.setDefault(a.id);
      revs.bump('accounts');
    } catch (e) {
      session.fail('Make default', e);
    }
  }
  async function remove(a: Account) {
    more = null;
    try {
      await api.accounts.remove(a.id);
      revs.bump('accounts');
    } catch (e) {
      session.fail('Remove account', e);
    }
  }

  const detail = (a: Account) => a.detail || (a.refreshedAt ? `Microsoft account, refreshed automatically ${since(a.refreshedAt)}` : 'Microsoft account');

  const STATUS: Record<Account['status'], { label: string; color: string }> = {
    valid: { label: 'Valid token', color: 'var(--ok)' },
    expired: { label: 'Expired token', color: 'var(--warn)' },
    signin: { label: 'Refresh failed', color: 'var(--danger)' },
    offline: { label: 'Local', color: 'var(--faint)' },
  };
</script>

<div class="scroll">
  <div class="page page-narrow" style="gap: 18px">
    <div style="display: flex; align-items: center; gap: 8px; height: 32px"><h1 class="h1" style="margin-right: auto">Accounts</h1><button class="btn btn-accent" onclick={() => session.open({ kind: 'add-account' })}>Add account</button></div>
    <div class="list">
      {#each accounts.data ?? [] as a (a.id)}
        {@const st = STATUS[a.status]}
        <div class="row ar">
          <span class="head" aria-hidden="true" style="background: {avatar(a, 40)}"></span>
          <span style="min-width: 0"><span style="display: flex; gap: 8px; align-items: center"><span class="trunc" style="font-weight: 500">{a.name}</span>{#if a.isDefault}<span class="badge badge-accent">Default</span>{/if}</span><span class="trunc small faint" style="display: block">{detail(a)}</span></span>
          <span>{#if a.status === 'signin'}<button class="btn btn-sm btn-accent" onclick={() => session.open({ kind: 'add-account', mode: 'browser', accountId: a.id })}>Sign in again</button>{/if}</span>
          <span class="small opt" style="gap: 6px; align-items: center; justify-content: flex-end; color: {st.color}"><span class="dot" style="background: {st.color}" aria-hidden="true"></span>{st.label}</span>
          <button class="btn btn-sm btn-icon btn-ghost" aria-label="More for {a.name}" aria-haspopup="menu" onclick={(e) => openMore(a, e)}><Icon name="more" size={14} /></button>
        </div>
      {:else}
        {#if accounts.data}
          <Empty text="No accounts yet. Sign in with Microsoft, or add a local account to play now." action="Add account" onclick={() => session.open({ kind: 'add-account' })} />
        {:else}
        {#each { length: skeletonRows('accounts', 3) } as _, k (k)}
          <div class="row ar" aria-hidden="true"><div class="sk" style="width: 40px; height: 40px; border-radius: 8px"></div><div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: 30%"></div><div class="sk" style="height: 10px; width: 55%"></div></div><span></span><div class="sk opt" style="height: 10px; width: 80px; justify-self: end"></div><div class="sk" style="height: 28px; width: 28px"></div></div>
        {/each}
        {/if}
      {/each}
    </div>
    <section class="card" aria-label="Account behaviour">
      <Setting title="Refresh sign-ins automatically" desc="Expired sessions are renewed in the background. An account only asks you to sign in again when that fails."><Toggle on={acc?.autoRefresh ?? true} label="Refresh sign-ins automatically" onchange={(v) => prefs.update((s) => (s.accounts.autoRefresh = v))} /></Setting>
      <div class="divider"></div>
      <Setting title="Play offline when sign-in fails" desc="Useful when Microsoft servers are down. Online servers will not accept you."><Toggle on={acc?.offlineFallback ?? true} label="Play offline when sign-in fails" onchange={(v) => prefs.update((s) => (s.accounts.offlineFallback = v))} /></Setting>
    </section>
    {#if more}
      {@const m = more}
      <Menu x={m.x} y={m.y} label="More for {m.account.name}" onclose={() => (more = null)}>
        {#if m.confirm}
          <MenuConfirm text="Remove {m.account.name}? {m.account.kind === 'microsoft' ? 'Its stored sign-in is deleted.' : ''}" action="Remove" oncancel={() => (more = null)} onconfirm={() => remove(m.account)} />
        {:else}
          {#if !m.account.isDefault}<button class="mi" role="menuitem" onclick={() => makeDefault(m.account)}><Icon name="check" size={14} /><span>Make default</span><span></span></button>{/if}
          <button class="mi" role="menuitem" onclick={() => (more = { ...m, confirm: true })}><Icon name="trash" size={14} /><span>Remove account</span><span></span></button>
        {/if}
      </Menu>
    {/if}
    <section class="card card-pad" aria-label="Security" style="display: grid; grid-template-columns: 20px minmax(0,1fr); gap: 12px">
      <Icon name="lock" size={18} sw={1.8} color="var(--accent)" />
      <span class="small muted">Sign-in tokens are kept in your system keychain, not in launcher files. Tokens are removed from logs before you share them.</span>
    </section>
  </div>
</div>

<style>
  .ar{grid-template-columns:40px minmax(0,1fr) auto 150px auto;height:68px}
  .ar .opt{display:flex}
  @media (max-width:1100px){.ar{grid-template-columns:40px minmax(0,1fr) auto auto}.ar .opt{display:none}}
  .head{width:40px;height:40px;border-radius:8px;overflow:hidden;image-rendering:pixelated}
</style>

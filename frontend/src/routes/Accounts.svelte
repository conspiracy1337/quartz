<script lang="ts">
  import { api } from '$lib/api';
  import { revs } from '$lib/revs.svelte';
  import type { Account } from '$lib/api/types';
  import { query } from '$lib/query.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';
  import Empty from '$lib/ui/Empty.svelte';
  import Menu from '$lib/ui/Menu.svelte';
  import { since } from '$lib/format';
  import { avatar, hasTexture } from '$lib/look';
  import { prefs } from '$lib/prefs.svelte';

  const accounts = query(() => api.accounts.list());
  const acc = $derived(prefs.value?.accounts);

  async function makeDefault(id: string) {
    await api.accounts.setDefault(id);
    revs.bump('accounts');
  }

  /** The row's "More" menu; removing asks once more inside it. */
  let more = $state<{ account: Account; x: number; y: number; confirm: boolean } | null>(null);
  function openMore(a: Account, e: MouseEvent) {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    more = { account: a, x: r.right - 240, y: r.bottom + 4, confirm: false };
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
    ok: { label: 'Signed in', color: 'var(--accent)' },
    expired: { label: 'Needs sign-in', color: 'var(--warn)' },
    offline: { label: 'Offline', color: 'var(--faint)' },
  };
</script>

<div class="scroll">
  <div class="page page-narrow" style="gap: 18px">
    <div style="display: flex; align-items: center; gap: 8px; height: 32px"><h1 class="h1" style="margin-right: auto">Accounts</h1><button class="btn btn-accent" onclick={() => session.open({ kind: 'add-account' })}>Add account</button></div>
    <div class="list">
      {#each accounts.data ?? [] as a (a.id)}
        {@const st = STATUS[a.status]}
        <div class="row ar">
          {#if hasTexture(a.skinHash)}<span class="head" aria-hidden="true" style="background: {avatar(a, 40)}"></span>{:else}<span class="head" aria-hidden="true"><span style="background: {a.skin.hair}"></span><span style="background: {a.skin.skin}"></span></span>{/if}
          <span style="min-width: 0"><span style="display: flex; gap: 8px; align-items: center"><span class="trunc" style="font-weight: 500">{a.name}</span>{#if a.isDefault}<span class="badge badge-accent">Default</span>{/if}</span><span class="trunc small faint" style="display: block">{detail(a)}</span></span>
          <span class="small opt" style="gap: 6px; align-items: center; color: {st.color}"><span class="dot" style="background: {st.color}" aria-hidden="true"></span>{st.label}</span>
          <span style="display: flex; gap: 6px">
            {#if !a.isDefault}<button class="btn btn-sm" onclick={() => makeDefault(a.id)}>Make default</button>{/if}
            {#if a.status === 'expired'}<button class="btn btn-sm btn-accent" onclick={() => session.open({ kind: 'add-account', mode: 'browser', accountId: a.id })}>Sign in again</button>{/if}
            <button class="btn btn-sm btn-icon btn-ghost" aria-label="More for {a.name}" aria-haspopup="menu" onclick={(e) => openMore(a, e)}><Icon name="more" size={14} /></button>
          </span>
        </div>
      {:else}
        {#if accounts.data}
          <Empty text="No accounts yet. Sign in with Microsoft, or add an offline account to play now." action="Add account" onclick={() => session.open({ kind: 'add-account' })} />
        {:else}
        {#each { length: 3 } as _, k (k)}
          <div class="row ar" aria-hidden="true"><div class="sk" style="width: 40px; height: 40px; border-radius: 8px"></div><div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: 30%"></div><div class="sk" style="height: 10px; width: 55%"></div></div><div class="sk opt" style="height: 10px; width: 80px"></div><div class="sk" style="height: 28px; width: 100px"></div></div>
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
          <div class="small muted menu-title">Remove {m.account.name}? {m.account.kind === 'microsoft' ? 'Its stored sign-in is deleted.' : ''}</div>
          <div style="display: flex; gap: 6px; padding: 4px 8px 6px"><button class="btn btn-sm" role="menuitem" onclick={() => (more = null)}>Cancel</button><button class="btn btn-sm btn-danger" role="menuitem" onclick={() => remove(m.account)}>Remove</button></div>
        {:else}
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
  .ar{grid-template-columns:40px minmax(0,1fr) 150px auto;height:68px}
  .ar .opt{display:flex}
  @media (max-width:1100px){.ar{grid-template-columns:40px minmax(0,1fr) auto}.ar .opt{display:none}}
  .head{width:40px;height:40px;border-radius:8px;display:grid;grid-template-rows:30% 70%;overflow:hidden;image-rendering:pixelated}
</style>

<script lang="ts">
  import { api } from '$lib/api';
  import { revs } from '$lib/revs.svelte';
  import type { Account } from '$lib/api/types';
  import { query } from '$lib/query.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';

  const accounts = query(() => api.accounts.list());

  async function makeDefault(id: string) {
    await api.accounts.setDefault(id);
    revs.bump('accounts');
  }
  let refresh = $state(true);
  let askAtLaunch = $state(false);
  let offlineFallback = $state(true);

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
          <span class="head" aria-hidden="true"><span style="background: {a.skin.hair}"></span><span style="background: {a.skin.skin}"></span></span>
          <span style="min-width: 0"><span style="display: flex; gap: 8px; align-items: center"><span class="trunc" style="font-weight: 500">{a.name}</span>{#if a.isDefault}<span class="badge badge-accent">Default</span>{/if}</span><span class="trunc small faint" style="display: block">{a.detail}</span></span>
          <span class="small opt" style="display: flex; gap: 6px; align-items: center; color: {st.color}"><span class="dot" style="background: {st.color}" aria-hidden="true"></span>{st.label}</span>
          <span style="display: flex; gap: 6px">
            {#if !a.isDefault}<button class="btn btn-sm" onclick={() => makeDefault(a.id)}>Make default</button>{/if}
            {#if a.status === 'expired'}<button class="btn btn-sm btn-accent" onclick={() => session.open({ kind: 'add-account', mode: 'browser' })}>Sign in again</button>{/if}
            <button class="btn btn-sm btn-icon btn-ghost" aria-label="More for {a.name}"><Icon name="more" size={14} /></button>
          </span>
        </div>
      {:else}
        {#each { length: 3 } as _, k (k)}
          <div class="row ar" aria-hidden="true"><div class="sk" style="width: 40px; height: 40px; border-radius: 8px"></div><div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: 30%"></div><div class="sk" style="height: 10px; width: 55%"></div></div><div class="sk opt" style="height: 10px; width: 80px"></div><div class="sk" style="height: 28px; width: 100px"></div></div>
        {/each}
      {/each}
    </div>
    <section class="card" aria-label="Account behaviour">
      <Setting title="Refresh sign-ins automatically" desc="Expired sessions are renewed in the background. An account only asks you to sign in again when that fails."><Toggle bind:on={refresh} label="Refresh sign-ins automatically" /></Setting>
      <div class="divider"></div>
      <Setting title="Ask which account to use at launch" desc="Otherwise the default account is used, or the one an instance overrides."><Toggle bind:on={askAtLaunch} label="Ask which account" /></Setting>
      <div class="divider"></div>
      <Setting title="Play offline when sign-in fails" desc="Useful when Microsoft servers are down. Online servers will not accept you."><Toggle bind:on={offlineFallback} label="Play offline when sign-in fails" /></Setting>
    </section>
    <section class="card card-pad" aria-label="Security" style="display: grid; grid-template-columns: 20px minmax(0,1fr); gap: 12px">
      <Icon name="lock" size={18} sw={1.8} color="var(--accent)" />
      <span class="small muted">Sign-in tokens are kept in your system keychain, not in launcher files. Tokens are removed from logs before you share them.</span>
    </section>
  </div>
</div>

<style>
  .ar{grid-template-columns:40px minmax(0,1fr) 150px auto;height:68px}
  @media (max-width:1100px){.ar{grid-template-columns:40px minmax(0,1fr) auto}.ar .opt{display:none}}
  .head{width:40px;height:40px;border-radius:8px;display:grid;grid-template-rows:30% 70%;overflow:hidden}
</style>

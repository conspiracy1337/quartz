<script lang="ts">
  import { untrack } from 'svelte';
  import { api } from '$lib/api';
  import { onEvent } from '$lib/bridge';
  import { keychainName } from '$lib/labels';
  import { query } from '$lib/query.svelte';
  import { go, href } from '$lib/router.svelte';
  import { session, type AddAccountMode } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import type { IconName } from '$lib/ui/icons';
  import SetupLayout from './SetupLayout.svelte';

  // Once an account is added (any of the three ways), setup moves on. The count is
  // read untracked: the accounts:changed bump must not re-run this and reset it.
  $effect(() => {
    let before = -1;
    void untrack(() => api.accounts.list()).then((l) => (before = l.length));
    return onEvent('accounts:changed', () => {
      void api.accounts.list().then((l) => {
        if (before >= 0 && l.length > before) go({ name: 'setup', step: 3 });
      });
    });
  });

  const info = query(() => api.app.info(), 'app.info');

  const OPTIONS: { mode: AddAccountMode; title: string; sub: string; icon: IconName; strong?: boolean }[] = [
    { mode: 'browser', title: 'Sign in with Microsoft', sub: 'Opens your browser', icon: 'microsoft', strong: true },
    { mode: 'code', title: 'Use a code on another device', sub: 'For Steam Deck, or when no browser is set', icon: 'phone' },
    { mode: 'offline', title: 'Play offline', sub: 'Pick a name. Online servers will not accept it.', icon: 'wifi-off' },
  ];
</script>

<SetupLayout step={2}>
  <div style="display: flex; flex-direction: column; gap: 8px">
    <h1 class="h1" style="font-size: 26px; line-height: 32px">Sign in to play</h1>
    <p class="muted" style="margin: 0">Use the Microsoft account that owns Minecraft: Java Edition.</p>
  </div>
  <div style="display: flex; flex-direction: column; gap: 8px">
    {#each OPTIONS as o (o.mode)}
      <button class="opt" onclick={() => session.open({ kind: 'add-account', mode: o.mode })}>
        <span class="oi" style={o.strong ? 'background: var(--text); color: var(--bg)' : undefined} aria-hidden="true"><Icon name={o.icon} size={16} sw={1.8} /></span>
        <span><span style="display: block; font-weight: 500">{o.title}</span><span class="small faint">{o.sub}</span></span>
        <Icon name="chevron-right" size={14} sw={2} />
      </button>
    {/each}
  </div>
  <div class="small faint" style="display: flex; gap: 8px; align-items: center"><Icon name="lock" size={14} sw={1.8} />Your sign-in is stored in {keychainName(info.data?.os)}.</div>
  <div style="display: flex; justify-content: space-between; align-items: center"><a href={href({ name: 'setup', step: 1 })} class="btn btn-ghost">Back</a><a href={href({ name: 'setup', step: 3 })} class="btn btn-ghost">Skip for now</a></div>
</SetupLayout>

<style>
  .opt{display:grid;grid-template-columns:36px minmax(0,1fr) 16px;gap:12px;align-items:center;min-height:64px;padding:10px 14px;border:1px solid var(--line-2);border-radius:10px;background:var(--panel);text-decoration:none;color:var(--text);text-align:left;width:100%}
  .opt:hover{background:var(--raise)}
  .opt .oi{width:36px;height:36px;border-radius:8px;background:var(--raise);display:grid;place-items:center;color:var(--muted)}
</style>

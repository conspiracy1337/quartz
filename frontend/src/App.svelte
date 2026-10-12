<script lang="ts">
  import { untrack } from 'svelte';
  import Sidebar, { type SideKey } from '$lib/components/Sidebar.svelte';
  import StatusBar from '$lib/components/StatusBar.svelte';
  import { installShortcuts } from '$lib/keys';
  import AddAccount from '$lib/overlays/AddAccount.svelte';
  import LaunchAs from '$lib/overlays/LaunchAs.svelte';
  import Blocked from '$lib/overlays/Blocked.svelte';
  import Crash from '$lib/overlays/Crash.svelte';
  import Host from '$lib/overlays/Host.svelte';
  import HostPanel from '$lib/overlays/HostPanel.svelte';
  import Install from '$lib/overlays/Install.svelte';
  import LaunchPanel from '$lib/overlays/LaunchPanel.svelte';
  import Palette from '$lib/overlays/Palette.svelte';
  import Shortcuts from '$lib/overlays/Shortcuts.svelte';
  import Toast from '$lib/overlays/Toast.svelte';
  import UpdateBanner from '$lib/overlays/UpdateBanner.svelte';
  import { applyAppearance, prefs } from '$lib/prefs.svelte';
  import { router, type Route } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Accounts from './routes/Accounts.svelte';
  import Browse from './routes/Browse.svelte';
  import Downloads from './routes/Downloads.svelte';
  import Home from './routes/Home.svelte';
  import Import from './routes/Import.svelte';
  import InstancePage from './routes/instance/InstancePage.svelte';
  import Library from './routes/Library.svelte';
  import NewInstance from './routes/NewInstance.svelte';
  import Project from './routes/Project.svelte';
  import About from './routes/settings/About.svelte';
  import Appearance from './routes/settings/Appearance.svelte';
  import General from './routes/settings/General.svelte';
  import Java from './routes/settings/Java.svelte';
  import Registry from './routes/settings/Registry.svelte';
  import Search from './routes/settings/Search.svelte';
  import Storage from './routes/settings/Storage.svelte';
  import Updates from './routes/settings/Updates.svelte';
  import ImportLaunchers from './routes/setup/ImportLaunchers.svelte';
  import SignIn from './routes/setup/SignIn.svelte';
  import Welcome from './routes/setup/Welcome.svelte';
  import Skins from './routes/Skins.svelte';
  import Syncing from './routes/Syncing.svelte';
  import { revs } from '$lib/revs.svelte';
  import { tasks } from '$lib/tasks.svelte';

  $effect(() => router.start());
  $effect(() => prefs.listen());
  $effect(() => tasks.listen());
  $effect(() => revs.listen());
  $effect(() => session.listen());
  $effect(() => applyAppearance());
  $effect(() => installShortcuts());
  void prefs.load();
  void tasks.load();
  void session.load();

  const route = $derived(router.route);
  // Dialogs belong to the screen they were opened on; navigating closes them.
  $effect(() => {
    void router.route;
    untrack(() => session.close());
  });
  const overlay = $derived(session.overlay);

  function sideKey(r: Route): SideKey {
    switch (r.name) {
      case 'new-instance': case 'import': return 'library';
      case 'project': return 'browse';
      case 'skins': case 'accounts': return 'account';
      case 'syncing': return 'sync';
      case 'settings': case 'settings-search': return 'settings';
      case 'instance': return 'instance';
      case 'setup': return 'home';
      default: return r.name;
    }
  }
</script>

{#snippet page(r: Route)}
  {#if r.name === 'home'}<Home />
  {:else if r.name === 'library'}<Library />
  {:else if r.name === 'new-instance'}<NewInstance />
  {:else if r.name === 'import'}<Import />
  {:else if r.name === 'browse'}<Browse />
  {:else if r.name === 'project'}<Project id={r.id} />
  {:else if r.name === 'skins'}<Skins />
  {:else if r.name === 'accounts'}<Accounts />
  {:else if r.name === 'syncing'}<Syncing />
  {:else if r.name === 'downloads'}<Downloads />
  {:else if r.name === 'settings-search'}<Search query={r.query} />
  {:else if r.name === 'settings'}
    {#if r.tab === 'appearance'}<Appearance />
    {:else if r.tab === 'java'}<Java />
    {:else if r.tab === 'storage'}<Storage />
    {:else if r.tab === 'updates'}<Updates />
    {:else if r.tab === 'about'}<About />
    {:else if r.tab === 'downloads' || r.tab === 'integrations'}<Registry tab={r.tab} />
    {:else}<General />{/if}
  {:else if r.name === 'instance'}
    {#key r.id}<InstancePage id={r.id} tab={r.tab} sub={r.sub} />{/key}
  {/if}
{/snippet}

<div class="app" style="position: relative">
  {#if route.name === 'setup'}
    {#if route.step === 1}<Welcome />{:else if route.step === 2}<SignIn />{:else}<ImportLaunchers />{/if}
  {:else}
    <div class="app-side"><Sidebar active={sideKey(route)} instanceId={route.name === 'instance' ? route.id : undefined} downloads={tasks.active.length} /></div>
    <main class="main">
      {#if session.updateReady && route.name !== 'home'}<UpdateBanner />{/if}
      {@render page(route)}
      <StatusBar />
    </main>
  {/if}

  {#if session.launch}<LaunchPanel instanceId={session.launch.instanceId} accountId={session.launch.accountId} stage={session.launch.stage} details={session.launch.details} pct={session.launch.pct} />{/if}
  {#each Object.entries(session.hosting) as [id, state] (id)}<HostPanel instanceId={id} {state} />{/each}
  {#if session.toast}<Toast {...session.toast} />{/if}
  {#if session.launchAs}{#key session.launchAs}<LaunchAs {...session.launchAs} />{/key}{/if}

  {#if overlay?.kind === 'add-account'}<AddAccount mode={overlay.mode} accountId={overlay.accountId} />
  {:else if overlay?.kind === 'install'}<Install projectId={overlay.projectId} />
  {:else if overlay?.kind === 'crash'}<Crash instanceId={overlay.instanceId} />
  {:else if overlay?.kind === 'blocked'}<Blocked />
  {:else if overlay?.kind === 'host'}<Host instanceId={overlay.instanceId} />
  {:else if overlay?.kind === 'palette'}{#key overlay.mode}<Palette mode={overlay.mode} />{/key}
  {:else if overlay?.kind === 'shortcuts'}<Shortcuts />
  {/if}
</div>

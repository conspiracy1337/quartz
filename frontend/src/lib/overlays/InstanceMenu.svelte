<script lang="ts">
  import { api } from '../api';
  import { duplicate, move, repair, trash } from '../instanceActions';
  import { avatar } from '../look';
  import { query } from '../query.svelte';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';
  import Menu from '../ui/Menu.svelte';

  /**
   * An instance's one menu, opened by right-click (Library, sidebar) and by the
   * header's ⋮: Play or Stop, Open folder, Host, the instance's actions, then
   * "Play as" another account once (the default doesn't change). Play as starts
   * another copy when the instance already runs.
   */
  interface Props { instanceId: string; x: number; y: number }
  let { instanceId, x, y }: Props = $props();

  const accounts = query(() => api.accounts.list());
  const instances = query(() => api.instances.list());
  const inst = $derived(instances.data?.find((i) => i.id === instanceId));
  const running = $derived(session.busy(instanceId));
  const host = $derived(session.hosting?.instanceId === instanceId ? session.hosting : null);
  const close = () => (session.instanceMenu = null);
  // Act first: closing the menu unmounts this component and its props with it.
  function act(fn: () => unknown) {
    void fn();
    close();
  }
</script>

<Menu {x} {y} label="{inst?.name ?? 'Instance'} actions" title={inst?.name} onclose={close}>
  {#if inst}
    {@const i = inst}
    {#if running}
      <button class="mi" role="menuitem" onclick={() => act(() => session.stop(i.id))}><Icon name="stop" size={14} /><span>Stop</span><span></span></button>
    {:else}
      <button class="mi" role="menuitem" onclick={() => act(() => session.play(i.id))}><Icon name="play" size={14} /><span>Play</span><span></span></button>
    {/if}
    <button class="mi" role="menuitem" onclick={() => act(() => api.instances.openFolder(i.id))}><Icon name="folder" size={14} /><span>Open folder</span><span></span></button>
    <!-- While its server starts or runs, Host brings its panel back instead of a second dialog. -->
    <button class="mi" role="menuitem" onclick={() => act(() => (host ? (session.hostPanel = true) : session.open({ kind: 'host', instanceId: i.id })))}><Icon name="server" size={14} /><span>{host ? 'Show server' : 'Host'}</span><span class="small faint">{host?.state === 'running' ? host.address : ''}</span></button>
    <div class="divider" style="margin: 4px 0"></div>
    <button class="mi" role="menuitem" onclick={() => act(() => session.open({ kind: 'rename', instanceId: i.id }))}><Icon name="file" size={14} /><span>Rename</span><span></span></button>
    <button class="mi" role="menuitem" onclick={() => act(() => session.open({ kind: 'look', instanceId: i.id }))}><Icon name="camera" size={14} /><span>Appearance</span><span></span></button>
    <button class="mi" role="menuitem" onclick={() => act(() => duplicate(i))}><Icon name="plus" size={14} /><span>Duplicate</span><span></span></button>
    <button class="mi" role="menuitem" onclick={() => act(() => session.open({ kind: 'shortcut', instanceId: i.id }))}><Icon name="link" size={14} /><span>Create shortcut…</span><span></span></button>
    <button class="mi" role="menuitem" onclick={() => act(() => session.open({ kind: 'export', instanceId: i.id }))}><Icon name="upload" size={14} /><span>Export…</span><span></span></button>
    <button class="mi" role="menuitem" onclick={() => act(() => repair(i))}><Icon name="refresh" size={14} /><span>Repair game files</span><span></span></button>
    <button class="mi" role="menuitem" onclick={() => act(() => move(i))}><Icon name="folder" size={14} /><span>Move game folder</span><span></span></button>
    <button class="mi" role="menuitem" onclick={() => act(() => trash(i))}><Icon name="trash" size={14} /><span>Delete</span><span></span></button>
  {/if}
  <div class="divider" style="margin: 4px 0"></div>
  <div class="small faint menu-title trunc">Play as</div>
  {#each accounts.data ?? [] as a (a.id)}
    <button class="mi" role="menuitem" onclick={() => act(() => session.play(instanceId, a.isDefault ? '' : a.id))}>
      <span class="avatar la-av" aria-hidden="true" style="background: {avatar(a, 20)}"></span>
      <span class="trunc">{a.name}</span>
      <span class="small faint">{a.isDefault ? 'Default' : a.kind === 'offline' ? 'Offline' : ''}</span>
    </button>
  {:else}
    {#if accounts.data}<button class="mi" role="menuitem" onclick={() => act(() => session.open({ kind: 'add-account' }))}><span></span><span>Add account</span><span></span></button>{/if}
  {/each}
</Menu>

<style>
  .la-av{width:20px;height:20px;flex:0 0 20px;border-radius:4px;image-rendering:pixelated}
</style>

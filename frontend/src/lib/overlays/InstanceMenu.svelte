<script lang="ts">
  import { api } from '../api';
  import InstanceActions from '../components/InstanceActions.svelte';
  import { avatar } from '../look';
  import { query } from '../query.svelte';
  import { session } from '../session.svelte';
  import Menu from '../ui/Menu.svelte';

  /** Right-click on an instance: its actions, then "Play as" another account once (the default doesn't change). */
  interface Props { instanceId: string; x: number; y: number }
  let { instanceId, x, y }: Props = $props();

  const accounts = query(() => api.accounts.list());
  const instances = query(() => api.instances.list());
  const inst = $derived(instances.data?.find((i) => i.id === instanceId));
  const close = () => (session.launchAs = null);

  function play(accountId: string) {
    close();
    session.play(instanceId, accountId);
  }
</script>

<Menu {x} {y} label="{inst?.name ?? 'Instance'} actions" title={inst?.name} onclose={close}>
  {#if inst}<InstanceActions instance={inst} onpick={close} />{/if}
  <div class="divider" style="margin: 4px 0"></div>
  <div class="small faint menu-title trunc">Play as</div>
  {#each accounts.data ?? [] as a (a.id)}
    <button class="mi" role="menuitem" onclick={() => play(a.isDefault ? '' : a.id)}>
      <span class="avatar la-av" aria-hidden="true" style="background: {avatar(a, 20)}"></span>
      <span class="trunc">{a.name}</span>
      <span class="small faint">{a.isDefault ? 'Default' : a.kind === 'offline' ? 'Offline' : ''}</span>
    </button>
  {:else}
    {#if accounts.data}<button class="mi" role="menuitem" onclick={() => (close(), session.open({ kind: 'add-account' }))}><span></span><span>Add account</span><span></span></button>{/if}
  {/each}
</Menu>

<style>
  .la-av{width:20px;height:20px;flex:0 0 20px;border-radius:4px;image-rendering:pixelated}
</style>

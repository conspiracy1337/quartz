<script lang="ts">
  import { api } from '../api';
  import type { Instance } from '../api/types';
  import { move, repair, trash } from '../instanceActions';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';

  /**
   * One instance's actions as menu rows, shared by the header's "More actions" menu
   * and the right-click menu so both offer and word them the same. The header has
   * its own Play and folder buttons, so `header` leaves those two out.
   */
  interface Props { instance: Pick<Instance, 'id' | 'name'>; header?: boolean; onpick: () => void }
  let { instance: i, header = false, onpick }: Props = $props();

  const running = $derived(!!session.running[i.id]);
  // Act first: closing the menu unmounts this component and its props with it.
  function act(fn: () => unknown) {
    void fn();
    onpick();
  }
</script>

{#if !header}
  {#if running}
    <button class="mi" role="menuitem" onclick={() => act(() => session.stop(i.id))}><Icon name="stop" size={14} /><span>Stop</span><span></span></button>
  {:else}
    <button class="mi" role="menuitem" onclick={() => act(() => session.play(i.id))}><Icon name="play" size={14} /><span>Play</span><span></span></button>
  {/if}
{/if}
<button class="mi" role="menuitem" onclick={() => act(() => session.open({ kind: 'rename', instanceId: i.id }))}><Icon name="file" size={14} /><span>Rename</span><span></span></button>
<button class="mi" role="menuitem" onclick={() => act(() => session.open({ kind: 'icon', instanceId: i.id }))}><Icon name="camera" size={14} /><span>Change icon</span><span></span></button>
{#if !header}
  <button class="mi" role="menuitem" onclick={() => act(() => api.instances.openFolder(i.id))}><Icon name="folder" size={14} /><span>Open folder</span><span></span></button>
{/if}
<button class="mi" role="menuitem" onclick={() => act(() => session.open({ kind: 'shortcut', instanceId: i.id }))}><Icon name="link" size={14} /><span>Create shortcut…</span><span></span></button>
<button class="mi" role="menuitem" onclick={() => act(() => repair(i))}><Icon name="refresh" size={14} /><span>Repair game files</span><span></span></button>
<button class="mi" role="menuitem" onclick={() => act(() => move(i))}><Icon name="folder" size={14} /><span>Move game folder</span><span></span></button>
<button class="mi" role="menuitem" onclick={() => act(() => trash(i))}><Icon name="trash" size={14} /><span>Move to trash</span><span></span></button>

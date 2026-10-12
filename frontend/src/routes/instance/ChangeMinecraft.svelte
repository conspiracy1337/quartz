<script lang="ts">
  import { api } from '$lib/api';
  import type { Instance } from '$lib/api/types';
  import VersionPicker from '$lib/components/VersionPicker.svelte';
  import { switchVersion } from '$lib/instanceActions';
  import { query } from '$lib/query.svelte';
  import { href } from '$lib/router.svelte';
  import Icon from '$lib/ui/Icon.svelte';

  interface Props { instance: Instance }
  let { instance: i }: Props = $props();
  let sel = $state('');
  let busy = $state(false);
  const back = $derived(href({ name: 'instance', id: i.id, tab: 'version' }));
  const check = query(() => (sel && sel !== i.minecraft ? api.instances.compatibility(i.id, sel) : Promise.resolve(null)));
  const compat = $derived(!sel || sel === i.minecraft ? 'This is the current version.' : check.data ? `${check.data.matching} of ${check.data.total} mods have a matching version` : 'Mods will be checked before switching');

  // A loader stays and moves to its newest stable build for the new version; mods move with it.
  async function switchTo() {
    busy = true;
    await switchVersion(i, sel, i.loader?.kind ?? '', '', `Minecraft ${sel}`);
    busy = false;
  }
</script>

<div class="scroll">
  <div class="page" style="gap: 14px; padding-top: 16px">
    <div style="display: flex; align-items: center; gap: 8px"><a href={back} class="btn btn-sm btn-ghost"><Icon name="chevron-left" size={12} sw={2} />Back</a><h2 class="h2" style="margin-right: auto">Change Minecraft version</h2></div>
    <VersionPicker current={i.minecraft} onpick={(v) => (sel = v)} />
  </div>
</div>
<div class="pfoot">
  <span class="mono muted">{i.minecraft}</span><Icon name="arrow-right" size={12} sw={2} /><span class="mono">{sel}</span>
  <span class="small faint" style="margin-left: 8px">{compat}</span>
  <span style="margin-left: auto; display: flex; gap: 8px"><a href={back} class="btn">Cancel</a><button class="btn btn-accent" disabled={!sel || sel === i.minecraft || busy} onclick={switchTo}>Switch to {sel}</button></span>
</div>

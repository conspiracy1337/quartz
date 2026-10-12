<script lang="ts">
  import type { Instance } from '$lib/api/types';
  import VersionPicker from '$lib/components/VersionPicker.svelte';
  import { href } from '$lib/router.svelte';
  import Icon from '$lib/ui/Icon.svelte';

  interface Props { instance: Instance }
  let { instance: i }: Props = $props();
  let sel = $state('1.21.5');
  const back = $derived(href({ name: 'instance', id: i.id, tab: 'version' }));
  const family = (v: string) => v.split('.').slice(0, 2).join('.');
  const compat = $derived(sel === i.minecraft ? 'This is the current version.' : family(sel) === family(i.minecraft) ? '41 of 48 mods have a matching version' : 'Mods will be checked before switching');
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
  <span style="margin-left: auto; display: flex; gap: 8px"><a href={back} class="btn">Cancel</a><button class="btn btn-accent" disabled={sel === i.minecraft}>Switch to {sel}</button></span>
</div>

<script lang="ts">
  import type { Instance } from '$lib/api/types';
  import { href } from '$lib/router.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';

  interface Props { instance: Instance }
  let { instance: i }: Props = $props();
  const VARS = ['$INST_NAME', '$INST_DIR', '$INST_MC_DIR', '$INST_JAVA', '$EXIT_CODE'];
  let env = $state([{ name: '__GL_THREADED_OPTIMIZATIONS', value: '0' }, { name: 'MESA_GL_VERSION_OVERRIDE', value: '4.6' }]);
  let glfw = $state(false);
  let openal = $state(false);
</script>

<div class="scroll">
  <div class="page page-narrow" style="gap: 22px; padding-top: 18px">
    <div style="display: flex; align-items: center; gap: 8px"><a href={href({ name: 'instance', id: i.id, tab: 'settings' })} class="btn btn-sm btn-ghost"><Icon name="chevron-left" size={12} sw={2} />Settings</a><h2 class="h2">Commands and variables</h2></div>
    <section class="grp" aria-label="Commands">
      <h2 class="h3">Commands</h2>
      <div class="card card-pad" style="display: flex; flex-direction: column; gap: 14px">
        <label class="field"><span class="field-label">Before launch</span><span class="input"><input class="mono" value={'$HOME/scripts/backup.sh "$INST_DIR"'} /></span><span class="field-help">Runs and must finish before the game starts.</span></label>
        <label class="field"><span class="field-label">Wrapper</span><span class="input"><input class="mono" placeholder="for example: prime-run, gamemoderun" /></span></label>
        <label class="field"><span class="field-label">After exit</span><span class="input"><input class="mono" placeholder="Runs after the game closes" /></span></label>
        <div class="small faint" style="display: flex; flex-wrap: wrap; gap: 6px; align-items: center">Variables {#each VARS as v (v)}<span class="kbd">{v}</span>{/each}</div>
      </div>
    </section>
    <section class="grp" aria-label="Environment variables">
      <div style="display: flex; justify-content: space-between; align-items: center"><h2 class="h3">Environment variables</h2><button class="btn btn-sm" onclick={() => env.push({ name: '', value: '' })}>Add</button></div>
      <div class="list">
        <div class="row row-head env"><span>Name</span><span>Value</span><span></span></div>
        {#each env as e, k (k)}
          <div class="row env"><span class="input"><input class="mono" bind:value={e.name} aria-label="Name" /></span><span class="input"><input class="mono" bind:value={e.value} aria-label="Value" /></span><button class="btn btn-icon btn-sm btn-ghost" aria-label="Remove" onclick={() => env.splice(k, 1)}><Icon name="x" size={12} sw={2} /></button></div>
        {/each}
      </div>
    </section>
    <section class="grp" aria-label="Libraries">
      <h2 class="h3">Native libraries</h2>
      <div class="card">
        <Setting title="Use system GLFW" desc="Fixes some Wayland and ARM setups."><Toggle bind:on={glfw} label="Use system GLFW" /></Setting>
        <div class="divider"></div>
        <Setting title="Use system OpenAL"><Toggle bind:on={openal} label="Use system OpenAL" /></Setting>
      </div>
    </section>
  </div>
</div>

<style>
  .env{grid-template-columns:minmax(0,1fr) minmax(0,1.4fr) 28px;height:40px}
</style>

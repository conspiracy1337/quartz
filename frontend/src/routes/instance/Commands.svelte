<script lang="ts">
  import { api } from '$lib/api';
  import type { Instance } from '$lib/api/types';
  import { revs } from '$lib/revs.svelte';
  import { href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Setting from '$lib/ui/Setting.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';

  interface Props { instance: Instance }
  let { instance: i }: Props = $props();
  const VARS = ['$INST_NAME', '$INST_DIR', '$INST_MC_DIR', '$INST_JAVA', '$EXIT_CODE'];
  // svelte-ignore state_referenced_locally
  let c = $state($state.snapshot(i.settings.commands));

  // Saves the whole settings object Go holds, with these commands in it. Rows without a
  // name are kept on screen while being typed but not sent.
  async function save() {
    try {
      const commands = { ...$state.snapshot(c), env: c.env.filter((e) => e.name.trim()) };
      await api.instances.updateSettings(i.id, { ...$state.snapshot(i.settings), commands });
      revs.bump('instances');
    } catch (e) {
      session.fail('Commands', e);
    }
  }
</script>

<div class="scroll">
  <div class="page page-narrow" style="gap: 22px; padding-top: 18px">
    <div style="display: flex; align-items: center; gap: 8px"><a href={href({ name: 'instance', id: i.id, tab: 'settings' })} class="btn btn-sm btn-ghost"><Icon name="chevron-left" size={12} sw={2} />Settings</a><h2 class="h2">Commands and variables</h2></div>
    <section class="grp" aria-label="Commands">
      <h2 class="h3">Commands</h2>
      <div class="card card-pad" style="display: flex; flex-direction: column; gap: 14px">
        <label class="field"><span class="field-label">Before launch</span><span class="input"><input class="mono" bind:value={c.preLaunch} onchange={save} /></span><span class="field-help">Runs and must finish before the game starts.</span></label>
        <label class="field"><span class="field-label">Wrapper</span><span class="input"><input class="mono" placeholder="for example: prime-run, gamemoderun" bind:value={c.wrapper} onchange={save} /></span></label>
        <label class="field"><span class="field-label">After exit</span><span class="input"><input class="mono" placeholder="Runs after the game closes" bind:value={c.postExit} onchange={save} /></span></label>
        <div class="small faint" style="display: flex; flex-wrap: wrap; gap: 6px; align-items: center">Variables {#each VARS as v (v)}<span class="kbd">{v}</span>{/each}</div>
      </div>
    </section>
    <section class="grp" aria-label="Environment variables">
      <div style="display: flex; justify-content: space-between; align-items: center"><h2 class="h3">Environment variables</h2><button class="btn btn-sm" onclick={() => c.env.push({ name: '', value: '' })}>Add</button></div>
      <div class="list">
        <div class="row row-head env"><span>Name</span><span>Value</span><span></span></div>
        {#each c.env as e, k (k)}
          <div class="row env"><span class="input"><input class="mono" bind:value={e.name} aria-label="Name" onchange={save} /></span><span class="input"><input class="mono" bind:value={e.value} aria-label="Value" onchange={save} /></span><button class="btn btn-icon btn-sm btn-ghost" aria-label="Remove" onclick={() => { c.env.splice(k, 1); void save(); }}><Icon name="x" size={12} sw={2} /></button></div>
        {/each}
      </div>
    </section>
    <section class="grp" aria-label="Libraries">
      <h2 class="h3">Native libraries</h2>
      <div class="card">
        <Setting title="Use system GLFW" desc="Fixes some Wayland and ARM setups."><Toggle bind:on={c.systemGlfw} label="Use system GLFW" onchange={save} /></Setting>
        <div class="divider"></div>
        <Setting title="Use system OpenAL"><Toggle bind:on={c.systemOpenal} label="Use system OpenAL" onchange={save} /></Setting>
      </div>
    </section>
  </div>
</div>

<style>
  .env{grid-template-columns:minmax(0,1fr) minmax(0,1.4fr) 28px;height:40px}
</style>

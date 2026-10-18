<script lang="ts">
  import { api } from '$lib/api';
  import { prefs } from '$lib/prefs.svelte';
  import { query } from '$lib/query.svelte';
  import { go, href } from '$lib/router.svelte';
  import { session } from '$lib/session.svelte';
  import { registry } from '$lib/settings/registry';
  import SettingControl from '$lib/settings/SettingControl.svelte';
  import Logo from '$lib/ui/Logo.svelte';
  import SetupLayout from './SetupLayout.svelte';

  const info = query(() => api.app.info(), 'app.info');
  const LOOKS = [
    { theme: 'monolith', name: 'Dark', bg: 'linear-gradient(90deg,#0c0c0e 0 28%,#0a0a0b 28%)' },
    { theme: 'light', name: 'Light', bg: 'linear-gradient(90deg,#ededef 0 28%,#f6f6f7 28%)' },
    { theme: 'system', name: 'System', bg: 'linear-gradient(90deg,#0c0c0e 0 50%,#f6f6f7 50%)' },
  ];
  // The same setting as General, worded for someone who hasn't picked anything yet.
  const base = registry.find((d) => d.id === 'language')!.control;
  const language = base.kind === 'select'
    ? { ...base, options: base.options.map((o) => (o.value === 'system' ? { ...o, label: 'English (from your system)' } : o)) }
    : base;
  const theme = $derived(prefs.value?.appearance.theme);

  // Change only picks a folder, shown in the field until Continue moves the
  // data there: the launcher restarts and comes back to this step.
  let chosen = $state('');
  let moving = $state(false);
  async function changeFolder() {
    try {
      chosen = (await api.app.chooseDataFolder()) || chosen;
    } catch (e) {
      session.fail('Change the data folder', e);
    }
  }
  async function next(e: MouseEvent) {
    if (!chosen) return;
    e.preventDefault();
    moving = true;
    try {
      if (!(await api.app.applyDataFolder())) {
        chosen = '';
        go({ name: 'setup', step: 2 });
      }
    } catch (err) {
      session.fail('Move the data folder', err);
      moving = false;
    }
  }
</script>

<SetupLayout step={1}>
  <div style="display: flex; flex-direction: column; gap: 8px">
    <Logo />
    <h1 class="h1" style="font-size: 26px; line-height: 32px">Set up the launcher</h1>
    <p class="muted" style="margin: 0">Three quick steps. You can change all of this later in Settings.</p>
  </div>
  <div class="field"><span class="field-label">Language</span><span class="tall"><SettingControl control={language} label="Language" /></span></div>
  <label class="field"><span class="field-label">Where to keep instances</span><span style="display: flex; gap: 6px"><span class="input" style="flex: 1 1 auto; height: 36px"><input class="mono" value={chosen || (info.data?.dataDir ?? '')} readonly /></span><button class="btn" style="height: 36px" onclick={changeFolder} disabled={moving || !info.data || info.data.dataDirFixed}>Change</button></span><span class="field-help">About 200 MB per vanilla instance. Modpacks can need several GB.</span></label>
  <div class="field"><span class="field-label">Look</span>
    <div style="display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px" role="radiogroup" aria-label="Look">
      {#each LOOKS as l (l.theme)}<button class="tp" class:on={theme === l.theme} onclick={() => prefs.update((s) => (s.appearance.theme = l.theme))} role="radio" aria-checked={theme === l.theme}><span style="background: {l.bg}; display: block"></span><span class="lb">{l.name}</span></button>{/each}
    </div>
  </div>
  <div style="display: flex; justify-content: space-between; align-items: center"><span class="small faint">Step 1 of 3</span><a href={href({ name: 'setup', step: 2 })} class="btn btn-accent btn-lg" onclick={next} aria-disabled={moving}>Continue</a></div>
</SetupLayout>

<style>
  .tall :global(.sel){width:100%;height:36px}
  .tp{height:64px;border:1px solid var(--line-2);border-radius:10px;padding:0;overflow:hidden;display:grid;grid-template-rows:1fr 24px;background:transparent;color:var(--text)}
  .tp.on{border-color:var(--accent);box-shadow:0 0 0 1px var(--accent)}
  .tp .lb{font-size:12px;display:flex;align-items:center;justify-content:center;border-top:1px solid var(--line)}
</style>

<script lang="ts">
  import Swatches from '$lib/ui/Swatches.svelte';
  import { tick } from 'svelte';
  import { api } from '$lib/api';
  import { prefs } from '$lib/prefs.svelte';
  import { groupsFor } from '$lib/settings/registry';
  import SettingsGroup from '$lib/settings/SettingsGroup.svelte';
  import Toggle from '$lib/ui/Toggle.svelte';
  import SettingsTab from './SettingsTab.svelte';

  // Preview colours of each preset: background, sidebar, line, text.
  const THEMES = [
    { id: 'monolith', name: 'Monolith', bg: '#0a0a0b', side: '#0c0c0e', line: '#1e1e22', text: '#ededef' },
    { id: 'graphite', name: 'Graphite', bg: '#16171a', side: '#1b1c20', line: '#2a2c31', text: '#e4e5e8' },
    { id: 'oled', name: 'OLED', bg: '#000000', side: '#000000', line: '#1a1a1a', text: '#f5f5f5' },
    { id: 'light', name: 'Light', bg: '#f6f6f7', side: '#ededef', line: '#dcdce0', text: '#18181b' },
  ];
  const ACCENTS = ['#3ecf8e', '#60a5fa', '#a78bfa', '#f472b6', '#f59e0b', '#f87171', '#e5e7eb'];
  const TOKENS = ['--bg', '--panel', '--text', '--accent'];
  const HEX = /^#[0-9a-fA-F]{6}$/;

  const a = $derived(prefs.value?.appearance);
  let hex = $state('');
  $effect(() => {
    hex = a?.accent ?? '';
  });

  // Live token values, read from the page after a change is applied so they include the custom CSS.
  let tokens = $state<{ t: string; v: string }[]>([]);
  $effect(() => {
    void [a?.theme, a?.accent, prefs.customCSS.css];
    void tick().then(() => {
      const css = getComputedStyle(document.documentElement);
      tokens = TOKENS.map((t) => ({ t, v: css.getPropertyValue(t).trim() }));
    });
  });
  const value = (t: string) => tokens.find((x) => x.t === t)?.v ?? '';
  const example = $derived(`:root {\n  --bg: ${value('--bg')};\n  --panel: ${value('--panel')};\n  --accent: ${value('--accent')};\n  --radius: 6px;\n}\n.sidebar { width: 216px; }`);

  function setAccent(v: string) {
    if (HEX.test(v)) prefs.update((s) => (s.appearance.accent = v.toLowerCase()));
  }
</script>

<SettingsTab tab="appearance">
  <section aria-label="Theme" style="display: flex; flex-direction: column; gap: 10px">
    <h2 class="h3">Theme</h2>
    <div class="themes" role="radiogroup" aria-label="Theme">
      {#each THEMES as t (t.id)}
        <button class="th" class:on={a?.theme === t.id} onclick={() => prefs.update((s) => (s.appearance.theme = t.id))} role="radio" aria-checked={a?.theme === t.id}><span class="pv" style="background: {t.bg}"><span style="background: {t.side}; border-right: 1px solid {t.line}"></span><span style="padding: 10px; display: flex; flex-direction: column; gap: 6px"><span style="height: 6px; width: 60%; border-radius: 3px; background: {t.text}"></span><span style="height: 6px; width: 40%; border-radius: 3px; background: {t.line}"></span><span style="height: 14px; width: 36px; border-radius: 4px; background: var(--accent); margin-top: auto"></span></span></span><span class="lb">{t.name}</span></button>
      {/each}
    </div>
  </section>
  <section aria-label="Accent colour" style="display: flex; flex-direction: column; gap: 10px">
    <h2 class="h3">Accent colour</h2>
    <div style="display: flex; gap: 10px; align-items: center">
      <Swatches colors={ACCENTS} value={a?.accent ?? ''} label="Accent colour" onpick={setAccent} />
      <span class="input" style="width: 110px; margin-left: 8px"><span class="sr">Custom hex</span><input class="mono" bind:value={hex} onchange={() => setAccent(hex)} maxlength="7" /></span>
    </div>
  </section>
  {#each groupsFor('appearance') as g (g.group)}<SettingsGroup defs={g.defs} />{/each}
  <section aria-label="Custom CSS" style="display: flex; flex-direction: column; gap: 10px">
    <div style="display: flex; justify-content: space-between; align-items: center"><h2 class="h3">Custom CSS</h2><div style="display: flex; gap: 6px"><button class="btn btn-sm" onclick={() => prefs.pickCustomCSS()}>Load file</button><button class="btn btn-sm" onclick={() => api.settings.exportTheme(example)}>Export theme</button></div></div>
    <div class="card" style="display: flex; align-items: center; gap: 10px; padding: 10px 12px">
      <Toggle on={a?.customCss.enabled ?? false} label="Use custom CSS file" disabled={!a?.customCss.path} onchange={(v) => prefs.update((s) => (s.appearance.customCss.enabled = v))} />
      <span class="mono small trunc" style="flex: 1 1 auto">{a?.customCss.path || 'No file loaded'}</span>
      {#if prefs.customCSS.error}<span class="badge badge-danger">{prefs.customCSS.error}</span>{:else if a?.customCss.path}<span class="badge badge-accent">Reloads on save</span>{/if}
    </div>
    <textarea class="textarea" rows="7" aria-label="Custom CSS" readonly value={prefs.customCSS.css || example}></textarea>
    <div class="list">
      <div class="row row-head tok"><span></span><span>Token</span><span>Value</span></div>
      {#each tokens as { t, v } (t)}
        <div class="tok"><span style="width: 16px; height: 16px; border-radius: 4px; background: var({t}); border: 1px solid var(--line-2)"></span><span class="mono">{t}</span><span class="mono muted">{v}</span></div>
      {/each}
    </div>
  </section>
</SettingsTab>

<style>
  .themes{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px}
  .th{border-radius:10px;border:1px solid var(--line);padding:0;overflow:hidden;background:transparent;text-align:left;color:var(--text)}
  .th.on{border-color:var(--accent);box-shadow:0 0 0 1px var(--accent)}
  .th .pv{height:72px;display:grid;grid-template-columns:28% 1fr}
  .th .lb{height:32px;display:flex;align-items:center;padding:0 10px;font-size:12px;border-top:1px solid var(--line)}
  .tok{display:grid;grid-template-columns:20px minmax(0,1fr) 92px;gap:10px;align-items:center;height:36px;padding:0 12px}
</style>

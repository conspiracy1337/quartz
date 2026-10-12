// Global settings as reactive state, plus everything that applies them to the document:
// theme preset, accent, density, text size, motion and the user's custom CSS file.
import { api, type CustomCSSState, type Settings } from './api';
import { onEvent } from './bridge';

const HEX = /^#[0-9a-fA-F]{6}$/;
const DEFAULT_ACCENT = '#3ecf8e';

class Prefs {
  value = $state<Settings | null>(null);
  customCSS = $state<CustomCSSState>({ path: '', css: '', error: '' });
  #pending: Promise<unknown> = Promise.resolve();

  async load() {
    const [s, css] = await Promise.all([api.settings.get(), api.settings.customCSS()]);
    this.value = s;
    this.customCSS = css;
  }

  /** Applies a change locally right away, then saves it; Go returns the validated result. */
  update(change: (s: Settings) => void) {
    if (!this.value) return;
    const next = $state.snapshot(this.value) as Settings;
    change(next);
    this.value = next;
    this.#pending = this.#pending.then(() => api.settings.save(next)).then((stored) => {
      if (this.value === next) this.value = stored;
    });
  }

  async pickCustomCSS() {
    this.value = await api.settings.pickCustomCSS();
  }

  /** Subscribes to changes made elsewhere (another window, a file watcher). */
  listen(): () => void {
    const offSettings = onEvent<Settings>('settings:changed', (s) => (this.value = s));
    const offCSS = onEvent<CustomCSSState>('theme:custom-css', (c) => (this.customCSS = c));
    return () => {
      offSettings();
      offCSS();
    };
  }
}

export const prefs = new Prefs();

/** Dark or light text for a solid accent background, by relative luminance. */
export function inkFor(hex: string): string {
  const n = parseInt(hex.slice(1), 16);
  const lin = (c: number) => {
    const v = c / 255;
    return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  };
  const l = 0.2126 * lin((n >> 16) & 255) + 0.7152 * lin((n >> 8) & 255) + 0.0722 * lin(n & 255);
  // 0.179 is where black and white text have equal contrast (WCAG).
  return l > 0.179 ? '#04140c' : '#f5f5f6';
}

/** Keeps <html> attributes and the custom CSS <style> in step with the settings. */
export function applyAppearance(): () => void {
  const root = document.documentElement;
  const dark = window.matchMedia('(prefers-color-scheme: dark)');
  let systemDark = $state(dark.matches);
  const onScheme = (e: MediaQueryListEvent) => (systemDark = e.matches);
  dark.addEventListener('change', onScheme);

  // Generated token values go in the tokens layer; the user's CSS stays unlayered and last,
  // so it overrides every token whatever selectors it uses.
  const vars = document.createElement('style');
  vars.id = 'theme-vars';
  const style = document.createElement('style');
  style.id = 'user-css';
  document.head.append(vars, style);

  const stop = $effect.root(() => {
    $effect(() => {
      const a = prefs.value?.appearance;
      if (!a) return;
      root.dataset.theme = a.theme === 'system' ? (systemDark ? 'monolith' : 'light') : a.theme;
      root.dataset.density = a.density;
      root.dataset.text = a.textSize;
      root.dataset.motion = a.reduceMotion === 'on' ? 'reduce' : a.reduceMotion === 'off' ? 'full' : 'system';
      // Changes apply here before Go validates them, so the value is checked again before it
      // is written into CSS text: only #rrggbb can ever reach the stylesheet.
      const accent = HEX.test(a.accent) ? a.accent : DEFAULT_ACCENT;
      vars.textContent = `@layer tokens{:root{--accent:${accent};--accent-ink:${inkFor(accent)}}}`;
    });
    $effect(() => {
      const on = prefs.value?.appearance.customCss.enabled ?? false;
      // textContent never parses markup, so the file can only ever be CSS.
      style.textContent = on ? prefs.customCSS.css : '';
    });
  });

  return () => {
    stop();
    dark.removeEventListener('change', onScheme);
    vars.remove();
    style.remove();
  };
}

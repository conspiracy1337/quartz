// Global keyboard shortcuts, as listed in the keyboard shortcuts sheet (shortcuts.ts);
// the list keys live in listkeys.ts, on the lists themselves.
import { api } from './api';
import { prefs } from './prefs.svelte';
import { INSTANCE_TABS, go, router, type Route } from './router.svelte';
import { session } from './session.svelte';
import { chordOf, chords, type ShortcutId } from './shortcuts';

const GO_TO: Record<string, Route> = { H: { name: 'home' }, L: { name: 'library' }, B: { name: 'browse' }, D: { name: 'downloads' } };
const SEQUENCE_MS = 800;

export const typing = (t: EventTarget | null) =>
  t instanceof HTMLElement && (t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName));

/** Runs a shortcut; false when it doesn't apply here. */
function run(id: ShortcutId): boolean {
  const r = router.route;
  const inst = r.name === 'instance' ? r : null;
  switch (id) {
    case 'palette': session.open({ kind: 'palette', mode: 'commands' }); return true;
    case 'switcher': session.open({ kind: 'palette', mode: 'instances' }); return true;
    case 'settings': go({ name: 'settings', tab: 'general' }); return true;
    case 'help': session.open({ kind: 'shortcuts' }); return true;
    case 'play':
      if (!inst || session.running[inst.id]) return false;
      session.play(inst.id);
      return true;
    case 'stop':
      if (!inst || !session.running[inst.id]) return false;
      void session.stop(inst.id);
      return true;
    case 'folder':
      if (!inst) return false;
      void api.instances.openFolder(inst.id);
      return true;
    case 'nextTab': {
      if (!inst) return false;
      const i = INSTANCE_TABS.indexOf(inst.tab);
      go({ name: 'instance', id: inst.id, tab: INSTANCE_TABS[(i + 1) % INSTANCE_TABS.length]! });
      return true;
    }
  }
  return false;
}

/** Installs the shortcuts; returns the uninstall function. */
export function installShortcuts(): () => void {
  let pendingG = 0;
  const onKey = (e: KeyboardEvent) => {
    const chord = chordOf(e);
    const mod = chord.startsWith('Ctrl+') || chord.startsWith('Alt+');
    const map = chords(prefs.value?.general.keys);
    const hit = (Object.keys(map) as ShortcutId[]).find((id) => map[id] === chord);
    // Chords with Ctrl or Alt work even while typing; bare keys don't.
    // With a dialog or the palette open, only the palette's own shortcuts apply.
    const allowed = hit && (!session.overlay || hit === 'palette' || hit === 'switcher');
    if (allowed && (mod || !typing(e.target)) && run(hit)) {
      e.preventDefault();
      return;
    }
    if (typing(e.target) || mod) return;
    if (e.key === 'Escape' && session.overlay) session.close();
    // Sequences go by chord too, so they work on any keyboard layout.
    else if (chord === 'G') pendingG = Date.now();
    else if (Date.now() - pendingG < SEQUENCE_MS && GO_TO[chord]) {
      pendingG = 0;
      go(GO_TO[chord]!);
    } else return;
    e.preventDefault();
  };
  window.addEventListener('keydown', onKey);
  return () => window.removeEventListener('keydown', onKey);
}

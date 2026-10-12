// Global keyboard shortcuts, as listed in the keyboard shortcuts sheet.
import { go, type Route } from './router.svelte';
import { session } from './session.svelte';

const GO_TO: Record<string, Route> = { h: { name: 'home' }, l: { name: 'library' }, b: { name: 'browse' }, d: { name: 'downloads' } };
const SEQUENCE_MS = 800;

const typing = (t: EventTarget | null) =>
  t instanceof HTMLElement && (t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName));

/** Installs the shortcuts; returns the uninstall function. */
export function installShortcuts(): () => void {
  let pendingG = 0;
  const onKey = (e: KeyboardEvent) => {
    const mod = e.ctrlKey || e.metaKey;
    if (mod && e.key.toLowerCase() === 'k') session.open({ kind: 'palette', mode: 'commands' });
    else if (mod && e.key.toLowerCase() === 'p') session.open({ kind: 'palette', mode: 'instances' });
    else if (mod && e.key === ',') go({ name: 'settings', tab: 'general' });
    else if (typing(e.target) || mod || e.altKey) return;
    else if (e.key === '?') session.open({ kind: 'shortcuts' });
    else if (e.key === 'Escape' && session.overlay) session.close();
    else if (e.key === 'g') pendingG = Date.now();
    else if (Date.now() - pendingG < SEQUENCE_MS && GO_TO[e.key]) {
      pendingG = 0;
      go(GO_TO[e.key]!);
    } else return;
    e.preventDefault();
  };
  window.addEventListener('keydown', onKey);
  return () => window.removeEventListener('keydown', onKey);
}

// "Press a key…" on a shared keybind: the next key or mouse button, named the way
// the browser does (KeyboardEvent.code, or "Mouse<button>"); Go maps it to the game.
import { chordOf } from './shortcuts';

/** The browser's name for a pressed key or mouse button. */
export const keyCode = (e: KeyboardEvent | MouseEvent): string => ('code' in e ? e.code : `Mouse${e.button}`);

/**
 * Waits for the next key or mouse button. Escape cancels (resolves ""). Listens in the
 * capture phase and stops the event, so global shortcuts and the page never see it.
 * With `chord`, it waits for a key with its modifiers ("Ctrl+K", see shortcuts.ts) and a
 * mouse button cancels.
 */
export function captureKey(chord = false): { done: Promise<string>; cancel: () => void } {
  let finish: (code: string) => void = () => {};
  const done = new Promise<string>((resolve) => (finish = resolve));
  const swallow = (e: Event) => {
    e.preventDefault();
    e.stopPropagation();
  };
  const onKey = (e: KeyboardEvent) => {
    swallow(e);
    if (e.key === 'Escape') return end('');
    if (!chord) return end(keyCode(e));
    const c = chordOf(e);
    if (c) end(c); // a lone modifier waits for its key
  };
  const onMouse = (e: MouseEvent) => {
    swallow(e);
    end(chord ? '' : keyCode(e));
  };
  function end(code: string) {
    window.removeEventListener('keydown', onKey, true);
    window.removeEventListener('mousedown', onMouse, true);
    window.removeEventListener('contextmenu', swallow, true);
    finish(code);
  }
  // The click that started listening is still on its way; take the next one.
  setTimeout(() => {
    window.addEventListener('keydown', onKey, true);
    window.addEventListener('mousedown', onMouse, true);
    window.addEventListener('contextmenu', swallow, true);
  });
  return { done, cancel: () => end('') };
}

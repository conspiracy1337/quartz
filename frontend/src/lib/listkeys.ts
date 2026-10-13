// The sheet's list keys on a .list of .row items: J and K move a cursor (the row's
// .sel state), Space flips the row's switch or checkbox, / focuses the page's search,
// Ctrl+A ticks every checkbox. Used as `use:listKeys` on the shared lists.
import { typing } from './keys';
import { session } from './session.svelte';

export function listKeys(node: HTMLElement) {
  let cur = -1;
  const rows = () => [...node.querySelectorAll<HTMLElement>(':scope > .row:not([aria-hidden="true"])')];
  const mark = (i: number) => {
    const all = rows();
    cur = Math.max(0, Math.min(i, all.length - 1));
    all.forEach((r, k) => r.classList.toggle('sel', k === cur));
    all[cur]?.scrollIntoView({ block: 'nearest' });
  };
  const onKey = (e: KeyboardEvent) => {
    if (session.overlay || typing(e.target) || e.altKey || !node.isConnected) return;
    const mod = e.ctrlKey || e.metaKey;
    if (!mod && (e.key === 'j' || e.key === 'k')) mark(cur + (e.key === 'j' ? 1 : -1));
    else if (!mod && e.key === ' ' && cur >= 0) rows()[cur]?.querySelector<HTMLElement>('.tg, .check')?.click();
    else if (!mod && e.key === '/') (node.closest('.page') ?? document).querySelector<HTMLInputElement>('input[type="search"]')?.focus();
    else if (mod && e.key.toLowerCase() === 'a') {
      const boxes = rows().map((r) => r.querySelector<HTMLElement>('button.check:not(.on)')).filter((b): b is HTMLElement => !!b);
      if (!node.querySelector('button.check')) return;
      boxes.forEach((b) => b.click());
    } else return;
    e.preventDefault();
  };
  window.addEventListener('keydown', onKey);
  return { destroy: () => window.removeEventListener('keydown', onKey) };
}

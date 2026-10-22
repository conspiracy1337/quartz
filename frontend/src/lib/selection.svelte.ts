import { pickRows } from './selection';
import { session } from './session.svelte';

/**
 * Rows picked for bulk actions, by the box before each row's ⋮ (the title row's box
 * picks every shown row): Ctrl-click a row ticks it, Shift-click picks the range from
 * the last one, Esc clears. order lists the shown rows' ids, top to bottom.
 */
export class Picker {
  picked = $state<Record<string, boolean>>({});
  private anchor: string | null = null;
  private order: () => string[];

  constructor(order: () => string[]) {
    this.order = order;
  }

  /** Every shown row is picked. */
  get all(): boolean {
    const ids = this.order();
    return ids.length > 0 && ids.every((id) => this.picked[id]);
  }

  pick(id: string, on: boolean, range = false) {
    this.anchor = pickRows(this.picked, this.order(), id, on, this.anchor, range);
  }

  pickAll(on: boolean) {
    for (const id of this.order()) this.picked[id] = on;
  }

  clear() {
    this.picked = {};
  }

  /** A click on a row with Ctrl (or Cmd) or Shift selects instead of doing what it hit. */
  click(e: MouseEvent, id: string) {
    if (!(e.ctrlKey || e.metaKey || e.shiftKey)) return;
    e.preventDefault();
    e.stopPropagation();
    this.pick(id, e.shiftKey || !this.picked[id], e.shiftKey);
  }

  /** Esc clears while something is picked; call from an $effect. */
  escape() {
    if (!Object.values(this.picked).some(Boolean)) return;
    const esc = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !session.overlay) this.clear();
    };
    window.addEventListener('keydown', esc);
    return () => window.removeEventListener('keydown', esc);
  }
}

/** Shift or Ctrl held down on a row: keep the press from selecting text. */
export const noTextPick = (e: MouseEvent) => (e.shiftKey || e.ctrlKey || e.metaKey) && e.preventDefault();

// Fuzzy matching for the command palette: the query's letters in order, not
// necessarily side by side ("ncs" finds "New instance… Create shortcut"), ranked so
// a plain substring beats scattered letters, and word starts and runs score higher.

export interface FuzzyMatch {
  score: number;
  /** Indexes of the matched characters, for highlighting. */
  at: number[];
}

const boundary = (s: string, i: number) => i === 0 || /[\s\-_.,:/()]/.test(s[i - 1]!) || (/[a-z]/.test(s[i - 1]!) && /[A-Z]/.test(s[i]!));

/** How well needle matches hay, or null when its letters aren't all there in order. */
export function fuzzy(needle: string, hay: string): FuzzyMatch | null {
  const n = needle.trim().toLowerCase();
  if (!n) return { score: 0, at: [] };
  const h = hay.toLowerCase();
  const sub = h.indexOf(n);
  if (sub >= 0) {
    // A substring: earlier and at a word start is better.
    return { score: 1000 + (boundary(hay, sub) ? 100 : 0) - sub, at: Array.from({ length: n.length }, (_, k) => sub + k) };
  }
  const at: number[] = [];
  let score = 0;
  let from = 0;
  for (const ch of n) {
    if (ch === ' ') continue;
    // Prefer the next word start holding this letter, else its next occurrence.
    let i = -1;
    for (let k = from; k < h.length; k++) {
      if (h[k] === ch && boundary(hay, k)) {
        i = k;
        break;
      }
    }
    const next = h.indexOf(ch, from);
    if (next < 0) return null;
    if (i < 0 || (at.length && next === at[at.length - 1]! + 1)) i = next;
    score += 10 + (boundary(hay, i) ? 8 : 0) + (at.length && i === at[at.length - 1]! + 1 ? 5 : 0) - Math.min(5, i - from);
    at.push(i);
    from = i + 1;
  }
  return { score, at };
}

/** The label cut into plain and matched runs, for <mark>. */
export function marks(label: string, at: number[]): { text: string; hit: boolean }[] {
  const hit = new Set(at);
  const out: { text: string; hit: boolean }[] = [];
  for (let i = 0; i < label.length; i++) {
    const on = hit.has(i);
    const last = out[out.length - 1];
    if (last && last.hit === on) last.text += label[i];
    else out.push({ text: label[i]!, hit: on });
  }
  return out;
}

/**
 * Ranks items by their label (fuzzy) and, below any label match, by extra text that
 * merely contains the query (descriptions, groups). Ties keep the items' order.
 */
export function rank<T>(items: readonly T[], needle: string, label: (t: T) => string, extra?: (t: T) => string): { item: T; at: number[] }[] {
  const q = needle.trim().toLowerCase();
  if (!q) return items.map((item) => ({ item, at: [] }));
  const found: { item: T; at: number[]; score: number; i: number }[] = [];
  items.forEach((item, i) => {
    const m = fuzzy(q, label(item));
    if (m) found.push({ item, at: m.at, score: m.score, i });
    else if (extra?.(item).toLowerCase().includes(q)) found.push({ item, at: [], score: -1, i });
  });
  return found.sort((a, b) => b.score - a.score || a.i - b.i).map(({ item, at }) => ({ item, at }));
}

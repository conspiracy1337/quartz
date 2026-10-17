// Loads async data for a component. Reactive values read inside `load` re-run it.
// While a reload is in flight the previous data stays visible, so the layout never
// falls back to a skeleton after the first load.

export interface Query<T> {
  readonly data: T | undefined;
  readonly loading: boolean;
  readonly error: unknown;
}

// A keyed query starts from its last answer in this run, so a screen opened again
// shows it at once while it refreshes. How many rows it had is also kept across runs,
// so the first skeleton has the loaded shape.
const last = new Map<string, unknown>();
const COUNT = 'quartz.rows.';

export function query<T>(load: () => Promise<T>, key?: string): Query<T> {
  const s = $state<{ data: T | undefined; loading: boolean; error: unknown }>({
    data: key === undefined ? undefined : (last.get(key) as T | undefined),
    loading: true,
    error: null,
  });
  $effect(() => {
    let stale = false;
    s.loading = true;
    load().then(
      (d) => {
        if (stale) return;
        s.data = d;
        s.error = null;
        s.loading = false;
        if (key !== undefined) {
          last.set(key, d);
          rememberCount(key, Array.isArray(d) ? d.length : d ? 1 : 0);
        }
      },
      (e: unknown) => {
        if (stale) return;
        s.error = e;
        s.loading = false;
      },
    );
    return () => {
      stale = true;
    };
  });
  return s;
}

/** Loads `key` ahead of its screen (unless it is in already), so the screen shows it at once. */
export function warm<T>(key: string, load: () => Promise<T>) {
  if (last.has(key)) return;
  load().then(
    (d) => void (last.has(key) || last.set(key, d)),
    () => {}, // the screen asks again when it opens
  );
}

/** How many rows (a list's length, or 1 for any other answer) `key` had last time, if known. */
export function lastCount(key: string): number | undefined {
  const d = last.get(key);
  if (d !== undefined) return Array.isArray(d) ? d.length : d ? 1 : 0;
  try {
    const n = localStorage.getItem(COUNT + key);
    return n === null ? undefined : Number(n) || 0;
  } catch {
    return undefined; // storage blocked: skeletons use their default
  }
}

export function rememberCount(key: string, n: number) {
  try {
    localStorage.setItem(COUNT + key, String(n));
  } catch {
    // storage blocked: the next run's skeleton uses its default
  }
}

/** Skeleton rows for a keyed list: as many as it had last time (at least 1), else `fallback`. */
export function skeletonRows(key: string, fallback: number): number {
  const n = lastCount(key);
  return n === undefined ? fallback : Math.max(1, n);
}

/** A row of filter chips as it looked: each chip's label and count (and its key, where the screen has one). */
export type ChipRow = { label: string; count: number; key?: string }[];
const CHIPS = 'quartz.chips.';

/** The chip row `key` showed last time, kept across runs, so it stands in place before its data is in. */
export function lastChips(key: string): ChipRow | undefined {
  try {
    const v: unknown = JSON.parse(localStorage.getItem(CHIPS + key) ?? 'null');
    const ok = Array.isArray(v) && v.every((c: Partial<ChipRow[number]> | null) =>
      typeof c?.label === 'string' && typeof c.count === 'number' && (c.key === undefined || typeof c.key === 'string'));
    return ok ? (v as ChipRow) : undefined;
  } catch {
    return undefined; // storage blocked or garbled: the chips wait for their data
  }
}

export function rememberChips(key: string, row: ChipRow) {
  try {
    localStorage.setItem(CHIPS + key, JSON.stringify(row));
  } catch {
    // storage blocked: the next run's chips wait for their data
  }
}

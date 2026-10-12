// Loads async data for a component. Reactive values read inside `load` re-run it.
// While a reload is in flight the previous data stays visible, so the layout never
// falls back to a skeleton after the first load.

export interface Query<T> {
  readonly data: T | undefined;
  readonly loading: boolean;
  readonly error: unknown;
}

export function query<T>(load: () => Promise<T>): Query<T> {
  const s = $state<{ data: T | undefined; loading: boolean; error: unknown }>({
    data: undefined,
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

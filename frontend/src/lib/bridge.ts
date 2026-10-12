// The Wails bridge exists only inside the desktop window. In a plain browser (vite dev,
// Playwright against vite, unit tests) it is absent and callers use their fallback.
import { EventsOn } from './wailsjs/runtime/runtime';

declare global {
  interface Window {
    go?: unknown;
    runtime?: unknown;
  }
}

export const hasBridge = (): boolean => typeof window !== 'undefined' && !!window.go && !!window.runtime;

/** Subscribes to a Go event; returns the unsubscribe function. A no-op without the bridge. */
export function onEvent<T>(topic: string, cb: (payload: T) => void): () => void {
  if (!hasBridge()) return () => {};
  return EventsOn(topic, (payload: T) => cb(payload));
}

// Open the app with ?slow to see every skeleton state in the preview.
const latency = typeof location !== 'undefined' && new URLSearchParams(location.search).has('slow') ? 1200 : 0;

/** Resolves preview data with a private copy (screens can never mutate fixtures), delayed under ?slow. */
export function preview<T>(value: T): Promise<T> {
  const copy = structuredClone(value);
  return latency ? new Promise((r) => setTimeout(() => r(copy), latency)) : Promise.resolve(copy);
}

/** Picks the Go binding inside the desktop window and the preview fallback elsewhere. */
export function bridged<A extends unknown[], R>(go: (...a: A) => Promise<unknown>, fallback: (...a: A) => R): (...a: A) => Promise<R> {
  return (...a: A) => (hasBridge() ? (go(...a) as Promise<R>) : preview(fallback(...a)));
}

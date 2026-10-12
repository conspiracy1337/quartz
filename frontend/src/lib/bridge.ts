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

/**
 * Picks the Go binding inside the desktop window and the preview fallback elsewhere.
 * The preview result is cloned so screens can never mutate fixture state.
 */
export function bridged<A extends unknown[], R>(go: (...a: A) => Promise<unknown>, preview: (...a: A) => R): (...a: A) => Promise<R> {
  return (...a: A) => (hasBridge() ? (go(...a) as Promise<R>) : Promise.resolve(structuredClone(preview(...a))));
}

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

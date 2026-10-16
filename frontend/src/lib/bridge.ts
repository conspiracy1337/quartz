// The Wails bridge exists only inside the desktop window. In a plain browser (vite dev,
// Playwright against vite, unit tests) it is absent and callers use their fallback.
// A production build only ever runs in the window, so there the check is constant and
// the bundler drops every preview fallback with its fixtures.
import { EventsOn } from './wailsjs/runtime/runtime';

declare global {
  interface Window {
    go?: unknown;
    runtime?: unknown;
  }
}

export const hasBridge = (): boolean => !import.meta.env.DEV || (typeof window !== 'undefined' && !!window.go && !!window.runtime);

/**
 * A preview-only object (a fixture-backed stand-in for a Go service). A production build
 * gets an empty object instead, so the bundler drops the stand-in and its fixtures; its
 * methods are only ever reached through bridged()'s fallback, which production never takes.
 */
export const previewStandIn = <T extends object>(make: () => T): T => (import.meta.env.DEV ? make() : ({} as T));

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

type Fn = (...a: never[]) => unknown;
type IsAny<T> = 0 extends 1 & T ? true : false;

declare const previewOnly: unique symbol;

/**
 * A field only the preview's fixtures fill (a painted stand-in for a picture).
 * Go never sends it, so it must be optional and its readers fall back; the
 * drift check lets it through.
 */
export type PreviewOnly<T> = T & { readonly [previewOnly]?: true };

/** T without its methods (a Pick, so optional fields stay optional). */
type Data<T> = Pick<T, { [K in keyof T]-?: T[K] extends Fn ? never : K }[keyof T]>;

/**
 * A Go value as it crosses the bridge: JSON data, so the generated classes'
 * methods (convertValues) don't count.
 */
export type Wire<T> =
  IsAny<T> extends true ? T
  : T extends Fn ? never
  : T extends readonly unknown[] ? { [K in keyof T]: Wire<T[K]> }
  : T extends object ? { [K in keyof Data<T>]: Wire<Data<T>[K]> }
  : T;

/** T with every field optional, at any depth: what Go accepts (a field left out is its zero value). */
type Loose<T> =
  IsAny<T> extends true ? T
  : T extends readonly unknown[] ? { [K in keyof T]?: Loose<T[K]> }
  : T extends object ? { [K in keyof T]?: Loose<T[K]> }
  : T;

/** V with `never` at every field G doesn't have, at any depth (V itself when there is none). */
type Known<V, G> =
  IsAny<G> extends true ? unknown
  : V extends Fn ? unknown
  : V extends readonly unknown[] ? (NonNullable<G> extends readonly unknown[] ? { [K in keyof V]: Known<V[K], NonNullable<G>[number]> } : unknown)
  : V extends object ? (NonNullable<G> extends object ? { [K in keyof V]: K extends keyof NonNullable<G> ? Known<V[K], NonNullable<G>[K]> : never } : unknown)
  : unknown;

/** T with string and number literal unions widened: Go's types only say string or number. */
type Widen<T> = T extends string ? string : T extends number ? number : T extends boolean ? boolean : T;

type IsUnion<T, U = T> = T extends unknown ? ([U] extends [T] ? false : true) : false;

/**
 * The paths in a preview result R that Go's result G doesn't back, or never:
 * a field Go lacks ("result.rows[].name") unless it is PreviewOnly, a field of another type, or a field
 * declared as always there that Go may leave out ("result.version?"). The
 * variants of a union (a block that is a heading or an image) each need only
 * fields Go has, since Go's one struct only sends a variant's own fields.
 */
type Drift<R, G, P extends string, Variant extends boolean = false> =
  IsAny<R> extends true ? never
  : IsAny<G> extends true ? never
  : R extends null | undefined ? never
  : R extends Fn ? never
  : R extends readonly unknown[]
    ? NonNullable<G> extends readonly unknown[] ? Drift<R[number], NonNullable<G>[number], `${P}[]`, IsUnion<NonNullable<R[number]>>> : P
  : R extends object
    ? NonNullable<G> extends readonly unknown[] ? P
      : NonNullable<G> extends Record<string, infer V> ? (string extends keyof NonNullable<G> ? Values<R, V, P> : Fields<R, One<NonNullable<G>>, P, Variant>)
      : NonNullable<G> extends object ? Fields<R, One<NonNullable<G>>, P, Variant>
      : P
  : [NonNullable<G>] extends [Widen<R>] ? never : P;

type Fields<R, G, P extends string, Variant extends boolean> = {
  [K in keyof R & string]-?: K extends keyof G
    ?
        | Drift<R[K], G[K], `${P}.${K}`, IsUnion<NonNullable<R[K]>>>
        | (Variant extends true ? never : undefined extends G[K] ? (undefined extends R[K] ? never : `${P}.${K}?`) : never)
    : typeof previewOnly extends keyof NonNullable<R[K]> ? (undefined extends R[K] ? never : `${P}.${K}`) : `${P}.${K}`;
}[keyof R & string];

/** A union of object types (a TS adapter's result) as one: every variant's fields, each optional. */
type Merge<G> = { [K in G extends unknown ? keyof G : never]?: G extends unknown ? (K extends keyof G ? G[K] : never) : never };
type One<G> = IsUnion<G> extends true ? Merge<G> : G;

/** Drift for the values of a Go map (whose keys the preview may name). */
type Values<R, V, P extends string> = { [K in keyof R & string]-?: Drift<R[K], V, `${P}.${K}`, IsUnion<NonNullable<R[K]>>> }[keyof R & string];

/**
 * Nothing when the preview's types fit Go's; otherwise a property the
 * fallback can't have, so the type error names what doesn't fit. Arguments
 * may use only fields Go reads (one left out arrives as its zero value); a
 * result may only declare what Go sends (see Drift).
 */
type Fit<A, GA, R, G> =
  [A] extends [Loose<Wire<GA>> & Known<A, Wire<GA>>]
    ? [Drift<R, Wire<G>, 'result', IsUnion<NonNullable<R>>>] extends [never]
      ? unknown
      : { previewDriftsFromGo: Drift<R, Wire<G>, 'result', IsUnion<NonNullable<R>>> & string } // & string lists the paths in the error
    : { previewArgumentsDontMatchGo: Known<A, Wire<GA>> };

/**
 * Picks the Go binding inside the desktop window and the preview fallback
 * elsewhere. The fallback's types are checked against the generated binding
 * (see Fit), so the preview and the screens can't drift from Go.
 */
export function bridged<GA extends unknown[], G, A extends unknown[], R>(
  go: (...a: GA) => Promise<G>,
  fallback: ((...a: A) => R) & Fit<A, GA, R, G>,
): (...a: A) => Promise<R> {
  const call = go as unknown as (...a: A) => Promise<R>;
  if (!import.meta.env.DEV) return call;
  return (...a: A) => (hasBridge() ? call(...a) : preview(fallback(...a)));
}

// Compile-time tests of bridged()'s drift check, run by `pnpm check` (never
// imported). Each @ts-expect-error must stay an error.
import { bridged, type PreviewOnly } from './bridge';

interface Art { background: string }
/** Shaped like a Wails class: data fields plus convertValues. */
interface GoRow { id: string; name: string; icon?: string; tags: string[]; counts: Record<string, number>; convertValues(a: unknown, c: unknown): unknown }
const get = (_id: string): Promise<GoRow> => Promise.resolve({} as GoRow);
const save = (_q: { text: string; limit: number }): Promise<void> => Promise.resolve();

export function typecheck() {
  // Fits: fewer fields, narrower strings, optional where Go always sends, named map keys, preview-only extras.
  bridged(get, (_id: string): { id: 'a' | 'b'; name?: string; icon?: string; counts: { mods: number } } => ({ id: 'a', counts: { mods: 1 } }));
  bridged(get, (_id: string): { id: string; art?: PreviewOnly<Art> } => ({ id: 'a', art: { background: 'red' } }));
  bridged(save, (_q: { text: string }) => undefined);

  // @ts-expect-error a field Go doesn't send
  bridged(get, (_id: string) => ({ id: 'a', renamed: 'x' }));
  // @ts-expect-error a field Go may leave out (omitempty), declared as always there
  bridged(get, (_id: string): { id: string; icon: string } => ({ id: 'a', icon: 'x' }));
  // @ts-expect-error a field of another type
  bridged(get, (_id: string) => ({ id: 1 }));
  // @ts-expect-error a nested field of another type
  bridged(get, (_id: string) => ({ tags: [1] }));
  // @ts-expect-error a preview-only field must be optional
  bridged(get, (_id: string): { id: string; art: PreviewOnly<Art> } => ({ id: 'a', art: { background: 'red' } }));
  // @ts-expect-error an argument field Go doesn't read
  bridged(save, (_q: { text: string; old: number }) => undefined);
  // @ts-expect-error an argument of another type
  bridged(save, (_q: { text: number }) => undefined);
  // @ts-expect-error a result where Go returns nothing
  bridged(save, (_q: { text: string }) => [1]);
}

// The keyboard shortcuts, defined once: the sheet (PalC) lists them, keys.ts handles
// them, and Settings, General edits the chords marked editable. A chord is written
// "Ctrl+K": modifiers first (Ctrl, Alt, Shift; Cmd counts as Ctrl), then the key.

export type ShortcutId = 'palette' | 'switcher' | 'settings' | 'help' | 'play' | 'stop' | 'folder' | 'nextTab';

export interface Shortcut { id: ShortcutId; label: string; chord: string; editable: boolean }
export interface ShortcutGroup { name: string; items: { label: string; keys: string[]; id?: ShortcutId }[] }

export const SHORTCUTS: readonly Shortcut[] = [
  { id: 'palette', label: 'Command palette', chord: 'Ctrl+K', editable: true },
  { id: 'switcher', label: 'Switch instance', chord: 'Ctrl+P', editable: true },
  { id: 'settings', label: 'Settings', chord: 'Ctrl+,', editable: true },
  { id: 'help', label: 'Show shortcuts', chord: '?', editable: false },
  { id: 'play', label: 'Play', chord: 'Ctrl+Enter', editable: true },
  { id: 'stop', label: 'Stop game', chord: 'Ctrl+.', editable: true },
  { id: 'folder', label: 'Open folder', chord: 'Ctrl+O', editable: true },
  { id: 'nextTab', label: 'Next tab', chord: 'Ctrl+Tab', editable: true },
];

/** The chords in effect: the defaults with the player's changes. */
export function chords(custom: Record<string, string> | undefined): Record<ShortcutId, string> {
  const out = {} as Record<ShortcutId, string>;
  for (const s of SHORTCUTS) out[s.id] = (s.editable && custom?.[s.id]) || s.chord;
  return out;
}

const NAMES: Record<string, string> = { ' ': 'Space', ArrowUp: 'Up', ArrowDown: 'Down', ArrowLeft: 'Left', ArrowRight: 'Right', Escape: 'Esc' };

/** The chord a key press makes ("" for a lone modifier). */
export function chordOf(e: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>): string {
  if (['Control', 'Meta', 'Alt', 'Shift'].includes(e.key)) return '';
  const key = NAMES[e.key] ?? (e.key.length === 1 ? e.key.toUpperCase() : e.key);
  const mods = [e.ctrlKey || e.metaKey ? 'Ctrl' : '', e.altKey ? 'Alt' : '', e.shiftKey && e.key.length > 1 ? 'Shift' : ''].filter(Boolean);
  return [...mods, key].join('+');
}

/** The keys of a chord, for the sheet's key caps. */
export const keysOf = (chord: string) => (chord === '+' ? ['+'] : chord.split('+').filter(Boolean).concat(chord.endsWith('++') ? ['+'] : []));

/** Why a chord can't be used for id (another shortcut has it, or it needs a modifier), or "". */
export function clash(id: ShortcutId, chord: string, current: Record<ShortcutId, string>): string {
  if (!chord.startsWith('Ctrl+') && !chord.startsWith('Alt+')) return 'Shortcuts need Ctrl or Alt';
  const other = SHORTCUTS.find((s) => s.id !== id && current[s.id] === chord);
  return other ? `${chord} is already ${other.label}` : '';
}

/** The sheet's groups (PalC), with the chords in effect. */
export function sheet(current: Record<ShortcutId, string>): ShortcutGroup[] {
  const pick = (...ids: ShortcutId[]) => ids.map((id) => ({ id, label: SHORTCUTS.find((s) => s.id === id)!.label, keys: keysOf(current[id]) }));
  return [
    { name: 'Anywhere', items: pick('palette', 'switcher', 'settings', 'help') },
    { name: 'Go to', items: [{ label: 'Home', keys: ['G', 'H'] }, { label: 'Library', keys: ['G', 'L'] }, { label: 'Browse', keys: ['G', 'B'] }, { label: 'Downloads', keys: ['G', 'D'] }] },
    { name: 'Instance', items: pick('play', 'stop', 'folder', 'nextTab') },
    { name: 'Lists', items: [{ label: 'Move', keys: ['J', 'K'] }, { label: 'Toggle selected', keys: ['Space'] }, { label: 'Search', keys: ['/'] }, { label: 'Select all', keys: ['Ctrl', 'A'] }] },
  ];
}

import type { Download, FinishedDownload } from '../types';

const MB = 1e6;
export const active: Download[] = [
  { id: 'd1', name: 'Cobblemon Official Modpack', sub: 'Installing as a new instance', art: { background: '#f2a25c', initial: 'C' }, doneBytes: 44 * MB, totalBytes: 71 * MB, etaSec: 40, queued: false, steps: [
    { name: 'Minecraft 1.21.1 and libraries', pct: 100, detail: 'Done', state: 'done' },
    { name: 'Fabric Loader 0.16.9', pct: 100, detail: 'Done', state: 'done' },
    { name: 'Mods', pct: 55, detail: '41 of 74', state: 'run' },
    { name: 'Assets', pct: 64, detail: '2,318 of 3,604', state: 'run' },
    { name: '1 mod needs a manual download', pct: 0, detail: 'Waiting', state: 'warn' },
  ] },
  { id: 'd2', name: 'Fabulously Optimized', sub: 'Updating 4 mods', art: { background: '#3ecf8e', initial: 'F' }, doneBytes: 0, totalBytes: 6.2 * MB, queued: true, steps: [
    { name: 'Sodium 0.6.5 to 0.6.6', pct: 0, detail: 'Queued', state: 'run' },
    { name: 'Lithium 0.14.3 to 0.14.4', pct: 0, detail: 'Queued', state: 'run' },
    { name: 'Iris Shaders 1.8.1 to 1.8.3', pct: 0, detail: 'Queued', state: 'run' },
    { name: 'Not Enough Animations 1.9.2 to 1.9.4', pct: 0, detail: 'Queued', state: 'run' },
  ] },
];

export const finished: FinishedDownload[] = [
  { id: 'f1', name: 'Iris Shaders 1.8.3', sub: 'Fabulously Optimized, 2 minutes ago', art: { background: '#b48cf2', initial: 'I' }, doneBytes: 2.4 * MB, totalBytes: 2.4 * MB, ok: true },
  { id: 'f2', name: 'Complementary Reimagined r5.4', sub: 'Fabulously Optimized, today', art: { background: '#f2b36b', initial: 'C' }, doneBytes: 1.1 * MB, totalBytes: 1.1 * MB, ok: true },
  { id: 'f3', name: 'Vanilla 1.21.4 game files', sub: 'Vanilla 1.21.4, yesterday', art: { background: '#6b8afd', initial: 'V' }, doneBytes: 188 * MB, totalBytes: 188 * MB, ok: true },
  { id: 'f4', name: 'Java 21.0.5 runtime', sub: 'Connection reset after 3 tries, 5 minutes ago', art: { background: '#5a5a63', initial: 'J' }, doneBytes: 48 * MB, totalBytes: 190 * MB, ok: false },
];

/** What the status bar tray shows while mod updates run. */
export const tray = {
  label: 'Updating 3 mods',
  pct: 41,
  speed: '3.2 MB/s',
  items: [
    { name: 'Iris Shaders 1.8.3', art: { background: '#b48cf2', initial: 'I' }, pct: 82 },
    { name: 'Not Enough Animations 1.9.4', art: { background: '#f2a25c', initial: 'N' }, pct: 40 },
    { name: 'YetAnotherConfigLib 3.6.2', art: { background: '#c9b6f2', initial: 'Y' }, pct: null as number | null },
  ],
};

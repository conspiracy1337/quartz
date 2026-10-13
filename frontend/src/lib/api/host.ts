// Hosting an instance as a local server: Go's host service shaped for the Host
// dialog and the server panel. The browser preview serves the DlgHost board's data.
import { bridged } from '../bridge';
import { ago, bytes } from '../format';
import { artOf, tintOf } from '../look';
import * as HostGo from '../wailsjs/go/api/Host';
import type { Art, LogLine } from './types';

type GoWorld = Awaited<ReturnType<typeof HostGo.Setup>>['worlds'][number];

/** One row of the dialog's world list. */
export interface HostWorld { folder: string; name: string; meta: string; size: string; art: Art }
/** How the server runs; world "" makes a new one. */
export interface HostOptions { world: string; port: number; maxPlayers: number; memoryMB: number; lan: boolean; offline: boolean }
export interface HostSetup { worlds: HostWorld[]; options: HostOptions; maxMemoryMB: number }
export interface HostMods { total: number; server: number; clientOnly: string[] }
export interface HostState {
  instanceId: string;
  runId: string;
  state: 'starting' | 'running' | 'stopped';
  stage?: string;
  port: number;
  /** This computer's address on the network, when the server is shown there. */
  lan?: string;
  players: number;
  max: number;
  result?: 'ok' | 'crash' | 'failed' | 'cancelled';
  message?: string;
}

/** "Survival, played yesterday" or "Creative, 2 weeks ago" (the board's wording). */
export function worldMeta(mode: string, lastPlayed: number, now = Date.now()): string {
  const a = ago(lastPlayed, now);
  return `${mode}, ${a === 'today' || a === 'yesterday' ? `played ${a}` : a}`;
}

/** A world without its own icon: a sky over land, tinted by its name. */
const worldArt = (w: GoWorld): Art =>
  w.icon ? artOf(w.name, w.icon) : { background: `linear-gradient(175deg,#7fb7d9 0 45%,${tintOf(w.name)} 45% 72%,#2c5236 72%)` };

const toWorld = (w: GoWorld): HostWorld => ({ folder: w.folder, name: w.name, meta: worldMeta(w.mode, w.lastPlayed), size: bytes(w.bytes), art: worldArt(w) });

const mockSetup = (): HostSetup => ({
  worlds: [
    { folder: 'Northwatch', name: 'Northwatch', meta: 'Survival, played yesterday', size: '1.8 GB', art: { background: 'linear-gradient(175deg,#7fb7d9 0 45%,#3f7d4f 45% 72%,#2c5236 72%)' } },
    { folder: 'Creative Plots', name: 'Creative Plots', meta: 'Creative, 2 weeks ago', size: '240 MB', art: { background: 'linear-gradient(175deg,#9ab3f0 0 45%,#6d7f9e 45% 72%,#3e4a5e 72%)' } },
  ],
  options: { world: 'Northwatch', port: 25565, maxPlayers: 10, memoryMB: 4096, lan: true, offline: false },
  maxMemoryMB: 32768,
});

const MOCK_CLIENT_ONLY = ['Sodium', 'Iris Shaders', 'Not Enough Animations', 'Entity Culling', 'Zoomify', 'Mod Menu', 'Sodium Extra', 'Reese’s Sodium Options',
  'Continuity', 'Indium', 'Lambdynamic Lights', 'Cull Leaves', 'Better F3', 'AppleSkin HUD', 'Xaero’s Minimap', 'Controlling', 'Chat Heads'];

const MOCK_CONSOLE: LogLine[] = [
  { time: '21:40:02', level: 'INFO', thread: 'Server thread', message: 'Done (4.218s)! For help, type "help"' },
  { time: '21:40:19', level: 'INFO', thread: 'Server thread', message: 'Kestrel_Vane joined the game' },
  { time: '21:40:31', level: 'INFO', thread: 'Server thread', message: 'Saved the game' },
];

export const hostApi = {
  setup: bridged(
    (id: string) => HostGo.Setup(id).then((s): HostSetup => ({ worlds: (s.worlds ?? []).map(toWorld), options: s.options, maxMemoryMB: s.maxMemoryMB })),
    (_id: string) => mockSetup(),
  ),
  /** Can ask Modrinth for mods it has no side for yet, so the dialog shows a skeleton meanwhile. */
  mods: bridged(HostGo.Mods, (_id: string): HostMods => ({ total: 48, server: 31, clientOnly: MOCK_CLIENT_ONLY })),
  start: bridged(HostGo.Start, (_id: string, _o: HostOptions) => undefined),
  stop: bridged(HostGo.Stop, (_id: string) => undefined),
  command: bridged(HostGo.Command, (_id: string, _line: string) => undefined),
  status: bridged(HostGo.Status, (): HostState[] => []),
  live: bridged(HostGo.Live, (_id: string): LogLine[] => MOCK_CONSOLE),
};

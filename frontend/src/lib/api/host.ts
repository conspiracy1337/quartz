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
/** How the server runs; world "" makes a new one (with seed, "" random). An empty motd is the
    instance's name, an empty difficulty the world's own, an empty icon Quartz's. */
export interface HostOptions {
  world: string; seed?: string; port: number; maxPlayers: number; memoryMB: number; viewDistance: number;
  motd: string; difficulty?: string; icon?: string; lan: boolean; offline: boolean; sandbox: boolean;
}
export interface HostSetup {
  worlds: HostWorld[]; options: HostOptions; maxMemoryMB: number;
  /** Each world's own difficulty and hardcore flag, by folder. */
  difficulty: Record<string, string>; hardcore: Record<string, boolean>;
  /** Servers can run in the sandbox here; this one must (its instance is sandboxed). */
  canSandbox: boolean; sandboxLocked: boolean;
}
/** One row of Manage: `on` runs it on the server. */
export interface HostMod { key: string; name: string; icon?: string; clientOnly: boolean; on: boolean }
/** server.properties for the editor, with Minecraft's defaults and the keys the dialog sets. */
export interface HostProperties { text: string; defaults: Record<string, string>; dialog: string[] }
export interface HostState {
  instanceId: string;
  runId: string;
  state: 'starting' | 'running' | 'stopped';
  stage?: string;
  port: number;
  /** This computer's address on the network, when the server is shown there. */
  lan?: string;
  /** Where players join: the network address with network joins on, else localhost (with the port). */
  address: string;
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
  options: { world: 'Northwatch', port: 25565, maxPlayers: 10, memoryMB: 4096, viewDistance: 10, motd: '', lan: true, offline: false, sandbox: false },
  maxMemoryMB: 32768,
  difficulty: { Northwatch: 'normal', 'Creative Plots': 'peaceful' },
  hardcore: {},
  canSandbox: true,
  sandboxLocked: false,
});

const MOCK_CLIENT_ONLY = new Set(['Sodium', 'Iris Shaders', 'Mod Menu', 'Xaero’s Minimap', 'Entity Culling', 'Continuity']);
const MOCK_MODS: HostMod[] = ['Fabric API', 'Sodium', 'Lithium', 'Iris Shaders', 'Create', 'Mod Menu', 'Xaero’s Minimap', 'Jade', 'Farmer’s Delight', 'Cloth Config API',
  'Waystones', 'Entity Culling', 'FerriteCore', 'Continuity', 'Supplementaries'].sort().map((name) => ({ key: `id:${name}`, name, clientOnly: MOCK_CLIENT_ONLY.has(name), on: !MOCK_CLIENT_ONLY.has(name) }));

const MOCK_PROPERTIES: HostProperties = {
  text: '#Minecraft server properties\ndifficulty=normal\nenable-command-block=false\nlevel-name=Northwatch\nmax-players=10\nmotd=Fabulously Optimized\npvp=true\nserver-port=25565\nspawn-protection=0\nview-distance=10\nwhite-list=false\n',
  defaults: { difficulty: 'easy', 'enable-command-block': 'false', 'level-name': 'world', 'max-players': '20', motd: 'A Minecraft Server', pvp: 'true', 'server-port': '25565', 'spawn-protection': '16', 'view-distance': '10', 'white-list': 'false' },
  dialog: ['view-distance', 'max-players', 'server-port'],
};

const MOCK_CONSOLE: LogLine[] = [
  { time: '21:40:02', level: 'INFO', thread: 'Server thread', message: 'Done (4.218s)! For help, type "help"' },
  { time: '21:40:19', level: 'INFO', thread: 'Server thread', message: 'Kestrel_Vane joined the game' },
  { time: '21:40:31', level: 'INFO', thread: 'Server thread', message: 'Saved the game' },
];

export const hostApi = {
  setup: bridged(
    (id: string) => HostGo.Setup(id).then((s): HostSetup => ({ ...s, worlds: (s.worlds ?? []).map(toWorld) })),
    (_id: string) => mockSetup(),
  ),
  /** Can ask Modrinth for mods it has no side for yet, so the dialog shows a skeleton meanwhile. */
  mods: bridged(HostGo.Mods, (_id: string): HostMod[] => MOCK_MODS.map((m) => ({ ...m }))),
  /** Keeps the Manage picks (mod key → runs on the server) for this instance's servers. */
  saveMods: bridged(HostGo.SaveMods, (_id: string, picks: Record<string, boolean>) => {
    for (const m of MOCK_MODS) if (m.key in picks) m.on = picks[m.key]!;
  }),
  properties: bridged(HostGo.Properties, (_id: string): HostProperties => ({ ...MOCK_PROPERTIES })),
  saveProperties: bridged(HostGo.SaveProperties, (_id: string, text: string) => {
    MOCK_PROPERTIES.text = text;
  }),
  /** Opens the instance's server folder (the same one every time it is hosted). */
  openFolder: bridged(HostGo.OpenFolder, (_id: string) => undefined),
  start: bridged(HostGo.Start, (_id: string, _o: HostOptions) => undefined),
  stop: bridged(HostGo.Stop, (_id: string) => undefined),
  command: bridged(HostGo.Command, (_id: string, _line: string) => undefined),
  status: bridged(HostGo.Status, (): HostState[] => []),
  live: bridged(HostGo.Live, (_id: string): LogLine[] => MOCK_CONSOLE),
};

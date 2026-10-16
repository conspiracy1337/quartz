import type { CrashInfo, LiveCopy, LogFile, LogLine, Run } from '../types';
import { daysAgo } from './time';

const L = (time: string, level: LogLine['level'], thread: string, message: string, redacted = false): LogLine =>
  ({ time, level, thread, message, ...(redacted ? { redacted } : {}) });

/** Two copies of the instance running, the second as another account. */
export const copies: LiveCopy[] = [
  { runId: '20261016-140211.000', accountId: 'a1', started: Date.now() - 42 * 60_000 },
  { runId: '20261016-143802.000', accountId: 'a2', started: Date.now() - 6 * 60_000 },
];

export const liveLog: LogLine[] = [
  L('21:14:02', 'INFO', 'main', 'Loading Minecraft 1.21.4 with Fabric Loader 0.16.10'),
  L('21:14:02', 'INFO', 'main', 'Loading 48 mods: sodium 0.6.5, lithium 0.14.3, iris 1.8.1, fabric-api 0.114.0 and 44 more'),
  L('21:14:03', 'INFO', 'main', 'SpongePowered MIXIN Subsystem Version=0.8.7 Source=union:/libraries/sponge-mixin.jar'),
  L('21:14:03', 'WARN', 'main', 'Mixin config entityculling.mixins.json does not specify "minVersion" property'),
  L('21:14:04', 'INFO', 'main', 'Setting user: Kestrel_Vane'),
  L('21:14:04', 'INFO', 'main', 'Session token ', true),
  L('21:14:06', 'INFO', 'Render thread', 'Backend library: LWJGL version 3.3.3-snapshot'),
  L('21:14:06', 'INFO', 'Render thread', '[Sodium] Using renderer: OpenGL 4.6, NVIDIA GeForce RTX 3070'),
  L('21:14:07', 'INFO', 'Render thread', '[Iris] Loaded shaderpack ComplementaryReimagined_r5.4.zip'),
  L('21:14:09', 'WARN', 'Render thread', 'Missing sound for event: minecraft:item.goat_horn.play'),
  L('21:14:11', 'INFO', 'Render thread', 'Created 2048x1024x4 minecraft:textures/atlas/blocks.png-atlas'),
  L('21:14:13', 'INFO', 'Server thread', 'Preparing level "Northwatch"'),
  L('21:14:15', 'INFO', 'Server thread', 'Loaded 1,402 advancements'),
  L('21:14:18', 'WARN', 'Server thread', "Can't keep up! Is the server overloaded? Running 2041ms or 40 ticks behind"),
  L('21:14:19', 'INFO', 'Render thread', '[DistantHorizons] LOD generation started, 512 chunk radius'),
  L('21:14:22', 'ERROR', 'Worker-Main-4', 'Failed to load texture minecraft:textures/entity/fox/snow_fox_sleep.png from pack "Faithful 32x"'),
  L('21:14:30', 'INFO', 'Render thread', 'Loaded 3 advancements for player Kestrel_Vane'),
  L('21:15:02', 'INFO', 'Server thread', 'Saving chunks for level ServerLevel[Northwatch]/minecraft:overworld'),
];

export const logFiles: LogFile[] = [
  { name: 'latest.log', bytes: 412_000, crash: false },
  { name: 'debug.log', bytes: 2_100_000, crash: false },
  { name: '2025-03-22-1.log.gz', bytes: 88_000, crash: false },
  { name: '2025-03-21-2.log.gz', bytes: 64_000, crash: false },
  { name: 'crash-2025-03-21_19.33.40-client.txt', bytes: 22_000, crash: true },
];

export const latestLogText = `[21:14:02] [main/INFO]: Loading Minecraft 1.21.4 with Fabric Loader 0.16.10
[21:14:02] [main/INFO]: Loading 48 mods:
	- fabric-api 0.114.0+1.21.4
	- iris 1.8.1+mc1.21.4
	- lithium 0.14.3
	- sodium 0.6.5+mc1.21.4
	- distanthorizons 2.3.0-b
[21:14:03] [main/INFO]: SpongePowered MIXIN Subsystem Version=0.8.7
[21:14:03] [main/WARN]: Mixin config entityculling.mixins.json does not specify "minVersion"
[21:14:04] [main/INFO]: Setting user: Kestrel_Vane
[21:14:06] [Render thread/INFO]: Backend library: LWJGL version 3.3.3
[21:14:06] [Render thread/INFO]: [Sodium] Using renderer: OpenGL 4.6
[21:14:07] [Render thread/INFO]: [Iris] Loaded shaderpack ComplementaryReimagined_r5.4.zip
[21:14:13] [Server thread/INFO]: Preparing level "Northwatch"
[21:14:22] [Worker-Main-4/ERROR]: Failed to load texture minecraft:textures/entity/fox/snow_fox_sleep.png
[21:15:02] [Server thread/INFO]: Saving chunks for level ServerLevel[Northwatch]
`;

/** What every sample run used. */
const used = { java: '21.0.5 (Temurin)', memory: { min: 2048, max: 6144 }, account: 'Kestrel_Vane', mods: 48, sandbox: 'bubblewrap' };

export const runs: Run[] = [
  { id: 'r1', startedAt: daysAgo(1, 21, 14), lengthMin: 128, exitCode: 0, result: 'ok', ...used },
  { id: 'r2', startedAt: daysAgo(2, 15, 2), lengthMin: 281, exitCode: 0, result: 'ok', ...used },
  { id: 'r3', startedAt: daysAgo(3, 19, 30), lengthMin: 3, exitCode: -1, result: 'crash', ...used },
  { id: 'r4', startedAt: daysAgo(3, 19, 22), lengthMin: 6, exitCode: 0, result: 'ok', ...used },
  { id: 'r5', startedAt: daysAgo(4, 22, 47), lengthMin: 72, exitCode: 137, result: 'killed', ...used },
  { id: 'r6', startedAt: daysAgo(24, 18, 5), lengthMin: 75, exitCode: 0, result: 'ok', ...used },
  { id: 'r7', startedAt: daysAgo(30, 23, 40), lengthMin: 0, exitCode: 1, result: 'failed', ...used },
  { id: 'r8', startedAt: daysAgo(40, 10, 12), lengthMin: 164, exitCode: 0, result: 'ok', ...used },
];

/** The design's crash dialog (DlgCrash). */
export const crash = (instanceId: string, runId: string): CrashInfo => ({
  instanceId, runId, lengthMin: 3, exitCode: 1,
  cause: {
    title: 'Entity Culling 1.7.2 failed to load',
    text: 'Its mixin into the entity renderer did not apply. This usually means it does not support Sodium 0.6.5 yet. Entity Culling 1.7.4 fixes this.',
    detail: 'Mixin apply failed in entityculling',
    mod: { item: 'ec', name: 'Entity Culling', version: '1.7.2', update: '1.7.4', enabled: true },
    fixes: ['update', 'disable'],
  },
  trace: `java.lang.RuntimeException: Mixin transformation of net.minecraft.client.render.entity.EntityRenderer failed
  at org.spongepowered.asm.mixin.transformer.MixinProcessor.applyMixins
  Caused by: InvalidInjectionException: @Inject(method = "shouldRender") on entityculling.mixins.json:EntityRendererMixin
  Mod file: mods/entityculling-fabric-1.7.2.jar`,
});

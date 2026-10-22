import type { Alert, DayPlay, MergedServer, NewsItem, Screenshot } from '../types';
import { daysAgo } from './time';

export const alerts: Alert[] = [
  { tone: 'warn', subject: 'BuilderBea', text: 'needs to sign in again', action: 'Sign in', target: { kind: 'sign-in', accountId: 'a3' } },
  { tone: 'warn', subject: 'Prominence II', text: 'is missing Sodium 0.6', action: 'Fix', target: { kind: 'instance', id: 'prom', tab: 'mods' } },
  { tone: 'info', subject: 'Create: Above and Beyond', text: '1.4.1 is out', action: 'Review', target: { kind: 'modpack-update', id: 'cab' } },
];

export const news: NewsItem = {
  title: 'Minecraft: Java Edition 26.2 is out',
  source: 'minecraft.net',
  published: daysAgo(26),
  url: 'https://www.minecraft.net/en-us/articles',
};

/** Minutes played per day, Monday to Sunday; the last entry is today. */
export const week: DayPlay[] = [85, 15, 150, 40, 160, 190, 60].map((minutes, i) => ({ day: i, minutes, today: i === 6 }));

const lg = (a: string, b: string) => ({ background: `linear-gradient(135deg,${a},${b})` });
export const servers: MergedServer[] = [
  { id: 's1', name: 'Friends SMP', address: 'smp.kestrel.gg', version: '1.21.4', fit: 'ok', online: true, players: { online: 6, max: 20 }, pingMs: 21, art: lg('#3ecf8e', '#1d6a4a'), instances: ['fo', 'van'] },
  { id: 's2', name: 'Hypixel', address: 'mc.hypixel.net', version: '1.21.4', fit: 'ok', online: true, players: { online: 41208, max: 200000 }, pingMs: 38, art: lg('#f2d45c', '#b8862f'), instances: ['van', 'fo', 'sky'] },
  { id: 's3', name: 'Cobblemon Academy', address: 'play.cobbleacademy.gg', version: '1.21.1', fit: 'ok', online: true, players: { online: 312, max: 500 }, pingMs: 64, art: lg('#f2a25c', '#8f4a1c'), instances: ['cob'] },
  { id: 's4', name: 'Skyblock Network', address: 'sb.islandsmc.net', version: '1.21.1', fit: 'minor', online: true, players: { online: 2940 }, pingMs: 96, art: lg('#5bc0be', '#1f5c5b'), instances: ['sky'] },
  { id: 's5', name: 'Modded Test', address: 'test.kestrel.gg', version: '1.20.1', fit: 'major', online: true, players: { online: 0, max: 10 }, pingMs: 182, art: lg('#b48cf2', '#5a3f8f'), instances: ['prom'] },
  { id: 's6', name: 'Old Creative Server', address: 'creative.kestrel.gg', version: '', fit: '', online: false, art: lg('#3a3a40', '#1a1a1e'), instances: ['fo'] },
];

const shot = (id: string, instanceId: string, world: string, title: string, at: number, bg: string): Screenshot => ({
  id, instanceId, world, title, takenAt: at, art: { background: `linear-gradient(175deg,${bg})` },
  file: `${new Date(at).toISOString().slice(0, 10)}_${String(new Date(at).getHours()).padStart(2, '0')}.${String(new Date(at).getMinutes()).padStart(2, '0')}.12.png`,
  width: 2560, height: 1440, bytes: 3_400_000,
});

/** Recent screenshots across every instance, newest first (Home rotator). */
export const recentShots: Screenshot[] = [
  shot('r1', 'fo', 'Northwatch', 'Northwatch at sunset', daysAgo(0, 20, 12), '#f2b36b 0 38%,#b85d4a 38% 48%,#2f4a3a 48% 78%,#1b2622 78%'),
  shot('r2', 'fo', 'Northwatch', 'The north farm', daysAgo(1, 22, 13), '#7fb7d9 0 45%,#3f7d4f 45% 70%,#2c5236 70%'),
  shot('r3', 'prom', 'Hastur', 'Hasturian ruins', daysAgo(2, 17, 20), '#3a2f5c 0 40%,#d97a8a 40% 52%,#1f3a3a 52% 80%,#0f1d1d 80%'),
  shot('r4', 'cob', 'Academy', 'First catch', daysAgo(3, 20, 11), '#9ec9f0 0 48%,#7d9b4f 48% 74%,#3f5a3f 74%'),
  shot('r5', 'van', 'Snowy', 'Snowy base', daysAgo(24, 19, 30), '#c9d6e3 0 42%,#eef3f7 42% 72%,#8fa1b3 72%'),
  shot('r6', 'sky', 'Islands', 'Nether hub', daysAgo(30, 23, 59), '#5a1f1f 0 40%,#b8432f 40% 62%,#3a1414 62%'),
];

const gridBgs = [
  '#f2b36b 0 38%,#b85d4a 38% 48%,#2f4a3a 48% 78%,#1b2622 78%',
  '#7fb7d9 0 45%,#3f7d4f 45% 70%,#2c5236 70%',
  '#1b2440 0 55%,#3a4a7a 55% 62%,#0f1424 62%',
  '#c9d6e3 0 42%,#eef3f7 42% 72%,#8fa1b3 72%',
  '#5a1f1f 0 40%,#b8432f 40% 62%,#3a1414 62%',
  '#6bb7d9 0 50%,#2f7fb0 50% 70%,#e6d39a 70%',
  '#3a2f5c 0 40%,#d97a8a 40% 52%,#1f3a3a 52% 80%,#0f1d1d 80%',
  '#9ec9f0 0 48%,#3f7d4f 48% 74%,#24402b 74%',
  '#f0c98a 0 55%,#7d9b4f 55% 75%,#5a4426 75%',
  '#2a2a40 0 30%,#7a8ac9 30% 34%,#2a2a40 34% 60%,#3f5a3f 60%',
  '#e8e0c9 0 50%,#c9b88a 50% 70%,#8a7a52 70%',
  '#0d1b2a 0 60%,#1b3a4a 60% 80%,#0a1218 80%',
];
const gridTimes: [number, number, number][] = [[1, 22, 41], [1, 22, 13], [1, 21, 58], [2, 17, 20], [2, 16, 2], [2, 15, 47], [3, 20, 11], [24, 19, 30], [24, 19, 2], [30, 23, 59], [33, 14, 15], [40, 10, 44]];

/** One instance's screenshots (instance Screenshots tab). */
export const instanceShots: Screenshot[] = gridBgs.map((bg, i) => {
  const [d, h, m] = gridTimes[i]!;
  return shot('g' + i, 'fo', i < 9 ? 'Northwatch' : 'Creative Plots', 'Screenshot', daysAgo(d, h, m), bg);
});

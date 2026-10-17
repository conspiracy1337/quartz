// Display formatting shared by every screen, so equal data always reads the same way.

const DAY = 86_400_000;
const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'];
const WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

const startOfDay = (t: number) => {
  const d = new Date(t);
  d.setHours(0, 0, 0, 0);
  return d.getTime();
};
const daysAgo = (t: number, now = Date.now()) => Math.round((startOfDay(now) - startOfDay(t)) / DAY);

/** "today", "yesterday", "Saturday" within a week, then "March 18". */
export function day(t: number, now = Date.now()): string {
  const d = daysAgo(t, now);
  if (d <= 0) return 'today';
  if (d === 1) return 'yesterday';
  if (d < 7) return WEEKDAYS[new Date(t).getDay()]!;
  return monthDay(t);
}

/** Same as day() but capitalised for the start of a label. */
export const Day = (t: number, now = Date.now()) => capitalize(day(t, now));

export const monthDay = (t: number) => {
  const d = new Date(t);
  return `${MONTHS[d.getMonth()]} ${d.getDate()}`;
};

export const shortMonthDay = (t: number) => {
  const d = new Date(t);
  return `${MONTHS[d.getMonth()]!.slice(0, 3)} ${d.getDate()}`;
};

export const fullDate = (t: number) => `${monthDay(t)}, ${new Date(t).getFullYear()}`;

/** Release and changelog dates: "Feb 28, 2025". */
export const shortDate = (t: number) => `${shortMonthDay(t)}, ${new Date(t).getFullYear()}`;

export const clock = (t: number) => {
  const d = new Date(t);
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
};

/** Compact age for cards: "Yesterday", "2 d ago", "1 w ago", "3 mo ago", "Never". */
export function agoShort(t: number | undefined, now = Date.now()): string {
  if (t === undefined) return 'Never';
  const d = daysAgo(t, now);
  if (d <= 0) return 'Today';
  if (d === 1) return 'Yesterday';
  if (d < 7) return `${d} d ago`;
  if (d < 30) return `${Math.round(d / 7)} w ago`;
  return `${Math.round(d / 30)} mo ago`;
}

/** Spelled-out age: "yesterday", "4 days ago", "3 weeks ago", "1 month ago". */
export function ago(t: number, now = Date.now()): string {
  const d = daysAgo(t, now);
  if (d <= 0) return 'today';
  if (d === 1) return 'yesterday';
  if (d < 7) return `${d} days ago`;
  if (d < 30) return plural(Math.round(d / 7), 'week') + ' ago';
  return plural(Math.round(d / 30), 'month') + ' ago';
}

/** Recent age with minute precision, for the last day: "just now", "5 minutes ago", "3 hours ago". */
export function since(t: number, now = Date.now()): string {
  const min = Math.floor((now - t) / 60_000);
  if (min < 1) return 'just now';
  if (min < 60) return plural(min, 'minute') + ' ago';
  if (min < 24 * 60) return plural(Math.floor(min / 60), 'hour') + ' ago';
  return ago(t, now);
}

/** Transfer rate: "12.4 MB/s". */
export const rate = (bytesPerSec: number) => `${bytes(bytesPerSec, 100)}/s`;

/** Time left: "40 s left", "3 min left"; empty while unknown. */
export const left = (sec: number) => (sec <= 0 ? '' : sec < 60 ? `${sec} s left` : `${Math.round(sec / 60)} min left`);

/** "Options, servers and shaders": names joined as a sentence, the first capitalised. */
export function sentenceList(names: string[]): string {
  const parts = names.map((n, i) => (i === 0 ? n.charAt(0).toUpperCase() + n.slice(1) : n));
  return parts.length <= 1 ? (parts[0] ?? '') : `${parts.slice(0, -1).join(', ')} and ${parts.at(-1)}`;
}
export const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;

// Separators follow the OS language (the WebView's navigator.language).
const TENTHS: Intl.NumberFormatOptions = { minimumFractionDigits: 1, maximumFractionDigits: 1 };
const tenths = new Intl.NumberFormat(undefined, TENTHS);

/** Time played, every total and session alike: "3,600.1 hours" ("3.600,1 hours" in German). Never rounds up. */
export function hours(min: number, locale?: string): string {
  const h = Math.floor(min / 6) / 10;
  return `${(locale ? new Intl.NumberFormat(locale, TENTHS) : tenths).format(h)} hours`;
}

const UNITS = ['B', 'KB', 'MB', 'GB', 'TB'];
/** File size with one decimal below `decimalsBelow` (10): "2.4 MB", "240 MB", "1.8 GB". */
export function bytes(n: number, decimalsBelow = 10): string {
  let i = 0;
  while (n >= 1000 && i < UNITS.length - 1) {
    n /= 1000;
    i++;
  }
  const v = n < decimalsBelow && i > 0 ? Math.round(n * 10) / 10 : Math.round(n);
  return `${v} ${UNITS[i]}`;
}

/** Memory is stored in MB and picked in whole GB. */
export const gb = (mb: number) => Math.round(mb / 1024);
export const mb = (gb: number) => gb * 1024;
/** A typed whole number of GB, in MB. */
export const gbField = (v: string) => mb(Math.max(0, Math.round(Number(v)) || 0));

export const count = (n: number) => n.toLocaleString('en-US');

/** "41,967 daily users": players in the last 24 hours. */
export const dailyUsers = (n: number) => `${count(n)} daily users`;

/** "6 / 20" players, or just "41,208" when the server names no limit. */
export const players = (p: { online: number; max?: number } | undefined) => (!p ? '' : p.max ? `${count(p.online)} / ${count(p.max)}` : count(p.online));

/** "58.7M", "312K". */
export function compact(n: number): string {
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)}M`;
  if (n >= 1e3) return `${Math.round(n / 1e3)}K`;
  return String(n);
}

export const percent = (part: number, whole: number) => (whole > 0 ? Math.round((part / whole) * 100) : 0);

export const capitalize = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

/** Search placeholder: "Search 48 mods", or "Search mods" while the count is unknown. */
export const searchIn = (noun: string, n?: number) => (n === undefined ? `Search ${noun}` : `Search ${n} ${noun}`);

/** A failed call's message for the UI: Go's "package: " prefixes dropped, first letter capitalised. */
export function errorText(err: unknown): string {
  const msg = (err instanceof Error ? err.message : String(err)).replace(/^([a-z]+: )+/, '');
  return msg.charAt(0).toUpperCase() + msg.slice(1);
}

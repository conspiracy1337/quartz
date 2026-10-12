// Fixture timestamps are relative to "now", so the mock always reads as recent activity.
const DAY = 86_400_000;

/** A moment `days` ago at the given local time. */
export function daysAgo(days: number, hh = 12, mm = 0): number {
  const d = new Date(Date.now() - days * DAY);
  d.setHours(hh, mm, 0, 0);
  return d.getTime();
}

/** A fixed calendar date (month is 1-based). */
export const date = (y: number, m: number, d: number) => new Date(y, m - 1, d, 12).getTime();

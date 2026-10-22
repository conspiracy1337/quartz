/** How long each item shows in Home's rotators. */
export const ROTATE_MS = 5000;

/**
 * Shows count() items one at a time, moving on every ms (Home's screenshot and news
 * rotators). It holds while paused (pointed at or focused) and while the window is
 * hidden (minimised, or the launcher hidden while playing). The component runs the timer:
 * `$effect(() => rot.start())`.
 */
export class Rotation {
  i = $state(0);
  paused = $state(false);
  private count: () => number;
  private ms: number;

  constructor(count: () => number, ms = ROTATE_MS) {
    this.count = count;
    this.ms = ms;
  }

  /** Starts the timer; returns what stops it. */
  start(): () => void {
    const t = setInterval(() => this.tick(), this.ms);
    return () => clearInterval(t);
  }

  /** The shown item, kept in range when items go away. */
  get at(): number {
    const n = this.count();
    return n > 0 ? this.i % n : 0;
  }

  step(by: number) {
    const n = this.count();
    if (n > 0) this.i = (((this.at + by) % n) + n) % n;
  }

  private tick() {
    if (!this.paused && !document.hidden) this.step(1);
  }
}

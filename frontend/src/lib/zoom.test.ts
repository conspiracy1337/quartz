import { describe, expect, it } from 'vitest';
import { FIT, MAX_ZOOM, clampView, fitted, pan, zoomAt } from './zoom';

const box = { w: 800, h: 600 };
const img = { w: 1600, h: 900 }; // fits as 800×450

describe('zoom', () => {
  it('fits the picture inside the box', () => {
    expect(fitted(img, box)).toEqual({ w: 800, h: 450 });
    expect(fitted({ w: 0, h: 0 }, box)).toEqual({ w: 0, h: 0 });
  });

  it('keeps the point under the cursor in place', () => {
    const v = zoomAt(FIT, 2, 600, 300, box, img);
    expect(v.s).toBe(2);
    // The box point 600,300 was 200px right of centre on the picture; it still is after zooming.
    const before = (600 - 400 - FIT.x) / FIT.s;
    const after = (600 - 400 - v.x) / v.s;
    expect(after).toBeCloseTo(before);
  });

  it('never zooms out past fit or in past the maximum', () => {
    expect(zoomAt(FIT, 0.5, 400, 300, box, img)).toEqual(FIT);
    expect(zoomAt({ s: MAX_ZOOM, x: 0, y: 0 }, 2, 400, 300, box, img).s).toBe(MAX_ZOOM);
  });

  it('pans only as far as the picture covers the box', () => {
    const v = { s: 2, x: 0, y: 0 }; // 1600×900 in an 800×600 box
    expect(pan(v, 10_000, 10_000, box, img)).toEqual({ s: 2, x: 400, y: 150 });
    expect(pan(v, -10_000, -10_000, box, img)).toEqual({ s: 2, x: -400, y: -150 });
  });

  it('centres an axis the picture does not fill', () => {
    // At 1.2× the picture is 960×540: wider than the box, still shorter than it.
    expect(clampView({ s: 1.2, x: 500, y: 80 }, box, img)).toEqual({ s: 1.2, x: 80, y: 0 });
  });
});

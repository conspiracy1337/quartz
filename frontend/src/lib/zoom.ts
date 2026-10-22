// Zooming and panning a picture inside a box. The picture first fits the box (contain,
// centred); a View scales that fitted picture about the box centre and shifts it.

export interface Size { w: number; h: number }
/** s is the scale over the fitted size (1 = fit); x and y shift the picture's centre, in px. */
export interface View { s: number; x: number; y: number }

export const FIT: View = { s: 1, x: 0, y: 0 };
export const MAX_ZOOM = 8;
/** One step of the zoom buttons and keys. */
export const ZOOM_STEP = 1.5;

/** The picture's size once it fits the box. */
export function fitted(img: Size, box: Size): Size {
  if (!img.w || !img.h) return { w: 0, h: 0 };
  const k = Math.min(box.w / img.w, box.h / img.h);
  return { w: img.w * k, h: img.h * k };
}

// A picture smaller than the box stays centred on that axis; a larger one may not uncover its edges.
const bound = (off: number, size: number, room: number) => {
  const max = Math.max(0, (size - room) / 2);
  return Math.min(max, Math.max(-max, off));
};

/** Keeps the scale in range and the picture covering the box wherever it is larger than it. */
export function clampView(v: View, box: Size, img: Size): View {
  const s = Math.min(MAX_ZOOM, Math.max(1, v.s));
  const f = fitted(img, box);
  return { s, x: bound(v.x, f.w * s, box.w), y: bound(v.y, f.h * s, box.h) };
}

/** Zooms by factor, keeping the point under (px, py) (box coordinates) where it is. */
export function zoomAt(v: View, factor: number, px: number, py: number, box: Size, img: Size): View {
  const s = Math.min(MAX_ZOOM, Math.max(1, v.s * factor));
  const cx = px - box.w / 2, cy = py - box.h / 2;
  const k = s / v.s;
  return clampView({ s, x: cx - (cx - v.x) * k, y: cy - (cy - v.y) * k }, box, img);
}

/** Shifts the picture by (dx, dy) from where it was. */
export const pan = (v: View, dx: number, dy: number, box: Size, img: Size): View => clampView({ s: v.s, x: v.x + dx, y: v.y + dy }, box, img);

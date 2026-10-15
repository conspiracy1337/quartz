// Minecraft's skin texture layout, for PlayerModel and the flat skin previews.

/** A texel offset [u, v] in a skin or cape texture. */
export type UV = [number, number];

/** The rectangle [u, v, width, height] each face of a w×h×d box at uv takes up, in skin3d's face order. */
export function faceRects([w, h, d]: [number, number, number], [u, v]: UV): [number, number, number, number][] {
  return [
    [u + d, v + d, w, h], // front
    [u + 2 * d + w, v + d, w, h], // back
    [u + d + w, v + d, d, h], // the player's left side
    [u, v + d, d, h], // the player's right side
    [u + d, v, w, d], // top
    [u + d + w, v, w, d], // bottom
  ];
}

/** A background showing texels (x, y, w, h) of a tw×th texture on a box of pw×ph pixels. */
export function crop(url: string, x: number, y: number, w: number, h: number, pw: number, ph: number, tw = 64, th = 64): string {
  const kx = pw / w, ky = ph / h;
  return `url("${url}") ${-x * kx}px ${-y * ky}px / ${tw * kx}px ${th * ky}px no-repeat`;
}

/** URL of a library skin's PNG (served by Go). */
export const skinURL = (hash: string) => `/tex/skin/${hash}.png`;
/** URL of a Minecraft texture (capes), fetched once and cached by Go. */
export const textureURL = (hash: string) => `/tex/mc/${hash}.png`;
/** URL of a default skin ("wide-steve", "slim-alex", …), embedded in Go. */
export const defaultURL = (key: string) => `/tex/default/${key}.png`;

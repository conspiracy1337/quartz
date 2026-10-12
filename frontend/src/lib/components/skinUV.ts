// Minecraft's skin texture layout, for PlayerModel and the flat skin previews.
import type { SkinColors } from '../api/types';

/** A texel offset [u, v] in a skin or cape texture. */
export type UV = [number, number];
/** A texture's size in texels. */
export type Size = [number, number];

/** Steve's colours: the flat drawing when there is neither a texture nor colours. */
export const STEVE: SkinColors = { skin: '#c69c6d', hair: '#3b2a1d', eye: '#4a3d8f', mouth: '#8b5a3c', top: '#00a3a3', topD: '#008585', pants: '#3d3a9e', shoes: '#4a4a4a' };

/** The rectangle [u, v, width, height] each face of a w×h×d box at uv takes up, in PlayerModel's face order. */
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

/**
 * CSS backgrounds that show one texture rectangle on a face. The face is `scale` px per texel
 * plus `inflate` texels (overlay layers), so the texture is stretched to fit exactly.
 */
export function uvBox(url: string, dims: [number, number, number], uv: UV, [tw, th]: Size, scale: number, inflate = 0): string[] {
  return faceRects(dims, uv).map(([x, y, w, h]) => crop(url, x, y, w, h, (w + inflate) * scale, (h + inflate) * scale, tw, th));
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

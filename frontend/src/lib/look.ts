// How accounts, library skins and capes are drawn: always from a real texture served by Go,
// the account's default skin (one of Minecraft's 18) when it wears none of its own.
import type { Account, Art, Cape, Skin } from './api/types';
import { crop, defaultURL, skinURL, textureURL } from './components/skinUV';

/** PlayerModel's skin props. */
export interface SkinLook { texture: string; slim: boolean }
/** PlayerModel's cape props. */
export interface CapeLook { capeTexture?: string | undefined }

/** Wide-armed Steve, while there is no account yet. */
export const STEVE: SkinLook = { texture: defaultURL('wide-steve'), slim: false };

/** A library skin. */
export const skinLook = (s: Pick<Skin, 'hash' | 'slim'>): SkinLook => ({ texture: skinURL(s.hash), slim: s.slim });

/** What an account wears, given the library (its skin may have left the library: the PNG stays). */
export function accountLook(a: Account, skins: Skin[] = []): SkinLook {
  if (!a.skinHash) return { texture: defaultURL(a.defaultSkin.key), slim: a.defaultSkin.slim };
  return skinLook(skins.find((s) => s.hash === a.skinHash) ?? { hash: a.skinHash, slim: a.slim });
}

export const capeLook = (c: Cape | undefined): CapeLook => (c?.texture ? { capeTexture: textureURL(c.texture) } : {});

/** A cape tile's picture: the outer side, 10×16 texels at (1, 1). */
export const capeTile = (c: Cape, w: number, h: number) => (c.texture ? crop(textureURL(c.texture), 1, 1, 10, 16, w, h, 64, 32) : (c.art ?? ''));

/** A head avatar of `px` pixels: the face with the hat layer on top. */
export function avatar(a: Account, px: number): string {
  const url = accountLook(a).texture;
  return `${crop(url, 40, 8, 8, 8, px, px)}, ${crop(url, 8, 8, 8, 8, px, px)}`;
}

/** Backgrounds of the flat front-view figure (head, arms, body, legs), one texel per pixel. */
export interface Figure { h: string; al: string; b: string; ar: string; ll: string; lr: string; arm: number }

export function figure({ texture: u, slim }: SkinLook): Figure {
  const arm = slim ? 3 : 4;
  // The viewer's left is the player's right. Each part: overlay layer over base layer.
  return {
    h: `${crop(u, 40, 8, 8, 8, 8, 8)}, ${crop(u, 8, 8, 8, 8, 8, 8)}`,
    al: `${crop(u, 44, 36, arm, 12, arm, 12)}, ${crop(u, 44, 20, arm, 12, arm, 12)}`,
    b: `${crop(u, 20, 36, 8, 12, 8, 12)}, ${crop(u, 20, 20, 8, 12, 8, 12)}`,
    ar: `${crop(u, 52, 52, arm, 12, arm, 12)}, ${crop(u, 36, 52, arm, 12, arm, 12)}`,
    ll: `${crop(u, 4, 36, 4, 12, 4, 12)}, ${crop(u, 4, 20, 4, 12, 4, 12)}`,
    lr: `${crop(u, 4, 52, 4, 12, 4, 12)}, ${crop(u, 20, 52, 4, 12, 4, 12)}`,
    arm,
  };
}

/** The design's instance tints (two near-duplicates left out); content without an icon reuses them. */
export const TINTS = ['#3ecf8e', '#e2a03f', '#6b8afd', '#b48cf2', '#c76bd8', '#9aa3ad', '#5bc0be', '#e86f6f', '#f2d45c', '#7cc7e8'];

/** A stable tint for a name, so an item keeps its colour between visits. */
export function tintOf(name: string): string {
  let h = 0;
  for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return TINTS[h % TINTS.length]!;
}

/** A picture served by Go (an item's or project's icon) over its tint, else the tint and an initial. */
export function artOf(name: string, src?: string): Art {
  const tint = tintOf(name);
  if (src && /^\/tex\/[a-z]+\/[0-9a-f]+\.png$/.test(src)) return { background: `url(${src}) center/cover no-repeat, ${tint}` };
  return { background: tint, initial: (/[\p{L}\p{N}]/u.exec(name)?.[0] ?? '?').toUpperCase() };
}

/** "6 GB suits 48 mods": about 2 GB plus 85 MB a mod, never more than half the PC's memory (0 when unknown). */
export const suggestMemoryGb = (mods: number, memoryMb: number) =>
  mods > 0 && memoryMb > 0 ? Math.min(Math.max(2, Math.round((2048 + mods * 85) / 1024)), Math.floor(memoryMb / 2048)) : 0;

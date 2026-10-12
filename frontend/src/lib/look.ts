// How accounts, library skins and capes are drawn: a real texture served by Go when
// there is one, the flat colour drawing otherwise (default skins and preview fixtures).
import type { Account, Cape, Skin, SkinColors } from './api/types';
import { STEVE, crop, skinURL, textureURL } from './components/skinUV';

/** PlayerModel's skin props. */
export interface SkinLook { colors: SkinColors; texture?: string | undefined; slim: boolean }
/** PlayerModel's cape props. */
export interface CapeLook { cape?: string | undefined; capeTexture?: string | undefined }

/** A library skin, falling back to `colors` while there is none. */
export function skinLook(s: Pick<Skin, 'hash' | 'slim' | 'colors'> | undefined, colors: SkinColors = STEVE): SkinLook {
  if (!s) return { colors, slim: false };
  return { colors: s.colors ?? colors, texture: s.colors ? undefined : skinURL(s.hash), slim: s.slim };
}

/** What an account wears, given the library (its skin may have left the library: the PNG stays). */
export function accountLook(a: Account, skins: Skin[] = []): SkinLook {
  if (!a.skinHash) return { colors: a.skin, slim: a.slim };
  return skinLook(skins.find((s) => s.hash === a.skinHash) ?? { hash: a.skinHash, slim: a.slim }, a.skin);
}

export const capeLook = (c: Cape | undefined): CapeLook => (!c ? {} : c.texture ? { capeTexture: textureURL(c.texture) } : { cape: c.art });

/** A cape tile's picture: the outer side, 10×16 texels at (1, 1). */
export const capeTile = (c: Cape, w: number, h: number) => (c.texture ? crop(textureURL(c.texture), 1, 1, 10, 16, w, h, 64, 32) : (c.art ?? ''));

/** A head avatar of `px` pixels: the face with the hat layer on top. */
export function avatar(a: Account, px: number): string {
  if (!a.skinHash) return `linear-gradient(${a.skin.hair} 0 38%, ${a.skin.skin} 38% 100%)`;
  const url = skinURL(a.skinHash);
  return `${crop(url, 40, 8, 8, 8, px, px)}, ${crop(url, 8, 8, 8, 8, px, px)}`;
}

/** Backgrounds of the flat front-view figure on skin tiles (head, arms, body, legs). */
export interface Figure { h: string; al: string; b: string; ar: string; ll: string; lr: string; arm: number }

export function figure(s: Pick<Skin, 'hash' | 'slim' | 'colors'>): Figure {
  const c = s.colors;
  if (c) {
    const sleeve = `linear-gradient(${c.top} 0 33%, ${c.skin} 33%)`;
    const leg = `linear-gradient(${c.pants} 0 75%, #1b1d22 75%)`;
    return { h: `linear-gradient(${c.hair} 0 30%, ${c.skin} 30%)`, al: sleeve, b: c.top, ar: sleeve, ll: leg, lr: leg, arm: 4 };
  }
  const u = skinURL(s.hash);
  const arm = s.slim ? 3 : 4;
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

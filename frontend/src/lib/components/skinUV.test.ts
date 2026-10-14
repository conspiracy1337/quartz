import { describe, expect, it } from 'vitest';
import { buildPlayer } from './skin3d';
import { crop, faceRects } from './skinUV';

describe('skin UV layout', () => {
  it('maps the head box like Minecraft', () => {
    // Head: 8×8×8 at (0, 0). Front (8,8), back (24,8), left side (16,8), right side (0,8), top (8,0), bottom (16,0).
    expect(faceRects([8, 8, 8], [0, 0])).toEqual([[8, 8, 8, 8], [24, 8, 8, 8], [16, 8, 8, 8], [0, 8, 8, 8], [8, 0, 8, 8], [16, 0, 8, 8]]);
  });

  it('maps a slim right arm', () => {
    const [front, back, left, right, top] = faceRects([3, 12, 4], [40, 16]);
    expect(front).toEqual([44, 20, 3, 12]);
    expect(back).toEqual([51, 20, 3, 12]);
    expect(left).toEqual([47, 20, 4, 12]);
    expect(right).toEqual([40, 20, 4, 12]);
    expect(top).toEqual([44, 16, 3, 4]);
  });

  it('scales the texture so one texel fills the face', () => {
    expect(crop('/s.png', 8, 8, 8, 8, 40, 40)).toBe('url("/s.png") -40px -40px / 320px 320px no-repeat');
  });

  it('puts every texture corner on the right corner of the 3D box', () => {
    const { data, parts } = buildPlayer(false);
    // Vertex k of a part: [x, y, z, u·64, v·64].
    const at = (part: string, k: number) => {
      const i = (parts[part]!.first + k) * 8;
      return [data[i], data[i + 1], data[i + 2], Math.round(data[i + 3]! * 64), Math.round(data[i + 4]! * (part === 'cape' ? 32 : 64))];
    };
    // Head front: the texture's top-left (8,8) is the viewer's top-left, facing the viewer (+z).
    expect(at('head', 0)).toEqual([-4, 8, 4, 8, 8]);
    // The player's right side (-x) keeps its front edge (u = 8) next to the face.
    expect(at('head', 6 * 3 + 1)).toEqual([-4, 8, 4, 8, 8]);
    // The hat layer is half a texel bigger on every side.
    expect(at('head+', 0)).toEqual([-4.5, 8.5, 4.5, 40, 8]);
    // Slim arms are 3 texels wide.
    const slim = buildPlayer(true);
    expect(slim.data[(slim.parts['rarm']!.first) * 8]).toBe(-1.5);
    // The cape's outer side (texture front, (1,1)) faces backwards, away from the body.
    expect(at('cape', 0)).toEqual([5, 0, -1, 1, 1]);
  });
});

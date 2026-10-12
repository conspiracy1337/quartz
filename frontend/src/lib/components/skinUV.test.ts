import { describe, expect, it } from 'vitest';
import { crop, faceRects, uvBox } from './skinUV';

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
    // A 0.5-texel inflated overlay face stretches to the larger box.
    const [front] = uvBox('/s.png', [8, 12, 4], [16, 32], [64, 64], 10, 0.5);
    expect(front).toBe(crop('/s.png', 20, 36, 8, 12, 85, 125));
  });
});

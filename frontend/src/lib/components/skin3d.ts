// A small WebGL player renderer for PlayerModel. The boxes and their texture layout are
// Minecraft's (as in Prism's skin preview), the shading is Minecraft's two-light entity
// lighting (as in Pandora's), and the overlay layer is alpha-tested so no sorting is needed.
// Units are texels; y is up, the player stands on y = 0 and faces +z.
import { faceRects, type UV } from './skinUV';

type Vec3 = [number, number, number];
type Mat = Float32Array;

/** What changes between frames. Angles are radians. */
export interface Pose {
  yaw: number;
  pitch: number;
  /** Limb swing (walk) and idle sway, as angles. */
  swing: number;
  sway: number;
  cape: number;
}

// ---- 4×4 column-major matrices, just what the scene needs ----
const ident = (): Mat => new Float32Array([1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1]);
function mul(a: Mat, b: Mat): Mat {
  const o = new Float32Array(16);
  for (let c = 0; c < 4; c++) for (let r = 0; r < 4; r++) {
    let s = 0;
    for (let k = 0; k < 4; k++) s += a[k * 4 + r]! * b[c * 4 + k]!;
    o[c * 4 + r] = s;
  }
  return o;
}
const move = ([x, y, z]: Vec3): Mat => { const m = ident(); m[12] = x; m[13] = y; m[14] = z; return m; };
function rot(axis: 0 | 1 | 2, a: number): Mat {
  const m = ident(), c = Math.cos(a), s = Math.sin(a);
  const [i, j] = axis === 0 ? [1, 2] : axis === 1 ? [2, 0] : [0, 1];
  m[i * 5] = c; m[j * 5] = c; m[i * 4 + j] = s; m[j * 4 + i] = -s;
  return m;
}
function perspective(fov: number, aspect: number, near: number, far: number): Mat {
  const f = 1 / Math.tan(fov / 2), m = new Float32Array(16);
  m[0] = f / aspect; m[5] = f; m[10] = (far + near) / (near - far); m[11] = -1; m[14] = (2 * far * near) / (near - far);
  return m;
}
const chain = (...ms: Mat[]) => ms.reduce(mul);

// ---- geometry ----
// Corners of each face in faceRects order (front, back, player's left, player's right, top,
// bottom), listed top-left, top-right, bottom-right, bottom-left as the texture shows them.
const CORNERS: Vec3[][] = [
  [[-1, 1, 1], [1, 1, 1], [1, -1, 1], [-1, -1, 1]],
  [[1, 1, -1], [-1, 1, -1], [-1, -1, -1], [1, -1, -1]],
  [[1, 1, 1], [1, 1, -1], [1, -1, -1], [1, -1, 1]],
  [[-1, 1, -1], [-1, 1, 1], [-1, -1, 1], [-1, -1, -1]],
  [[-1, 1, -1], [1, 1, -1], [1, 1, 1], [-1, 1, 1]],
  [[-1, -1, -1], [1, -1, -1], [1, -1, 1], [-1, -1, 1]],
];
const NORMALS: Vec3[] = [[0, 0, 1], [0, 0, -1], [1, 0, 0], [-1, 0, 0], [0, 1, 0], [0, -1, 0]];
/** Floats per vertex: position, uv, normal, and the face's texture rectangle (see `box`). */
export const STRIDE = 12;

/**
 * A w×h×d box at texture offset uv, grown by `grow` texels on every side, centred on `at`.
 * Each vertex also carries its face's texture rectangle, a quarter texel inside the edge: the
 * fragment shader clamps to it, so a sample taken just past a face's edge (multisampling does
 * that at silhouettes) never picks up the neighbouring texel as a thin line.
 */
function box(out: number[], [w, h, d]: Vec3, uv: UV, at: Vec3, tex: [number, number], grow = 0) {
  const half: Vec3 = [w / 2 + grow, h / 2 + grow, d / 2 + grow];
  const [tw, th] = tex;
  faceRects([w, h, d], uv).forEach(([x, y, fw, fh], f) => {
    const uvs = [[x, y], [x + fw, y], [x + fw, y + fh], [x, y + fh]].map(([u, v]) => [u! / tw, v! / th]);
    const rect = [(x + 0.25) / tw, (y + 0.25) / th, (x + fw - 0.25) / tw, (y + fh - 0.25) / th];
    for (const k of [0, 1, 2, 0, 2, 3]) {
      const c = CORNERS[f]![k]!;
      out.push(at[0] + c[0] * half[0], at[1] + c[1] * half[1], at[2] + c[2] * half[2], uvs[k]![0]!, uvs[k]![1]!, ...NORMALS[f]!, ...rect);
    }
  });
}

/** A body part: its base and overlay boxes, built around its pivot. */
interface Part { pivot: Vec3; first: number; count: number; layer: 'base' | 'over' | 'cape' | 'floor' }

/** The vertices of every part for classic or slim arms (exported for tests). */
export function buildPlayer(slim: boolean): { data: Float32Array; parts: Record<string, Part> } {
  const v: number[] = [];
  const parts: Record<string, Part> = {};
  const add = (name: string, layer: Part['layer'], pivot: Vec3, make: () => void) => {
    const first = v.length / STRIDE;
    make();
    parts[name] = { pivot, first, count: v.length / STRIDE - first, layer };
  };
  const skin: [number, number] = [64, 64];
  const aw = slim ? 3 : 4, ax = 4 + aw / 2;
  // [name, size, base uv, overlay uv, pivot, centre relative to the pivot, overlay growth]
  const layout: [string, Vec3, UV, UV, Vec3, Vec3, number][] = [
    ['head', [8, 8, 8], [0, 0], [32, 0], [0, 24, 0], [0, 4, 0], 0.5],
    ['body', [8, 12, 4], [16, 16], [16, 32], [0, 24, 0], [0, -6, 0], 0.25],
    ['rarm', [aw, 12, 4], [40, 16], [40, 32], [-ax, 22, 0], [0, -4, 0], 0.25],
    ['larm', [aw, 12, 4], [32, 48], [48, 48], [ax, 22, 0], [0, -4, 0], 0.25],
    ['rleg', [4, 12, 4], [0, 16], [0, 32], [-2, 12, 0], [0, -6, 0], 0.25],
    ['lleg', [4, 12, 4], [16, 48], [0, 48], [2, 12, 0], [0, -6, 0], 0.25],
  ];
  for (const [name, size, base, over, pivot, at, grow] of layout) {
    add(name, 'base', pivot, () => box(v, size, base, at, skin));
    add(name + '+', 'over', pivot, () => box(v, size, over, at, skin, grow));
  }
  // The cape hangs from the shoulders; its texture's "front" is the outer side, so the box is
  // built facing backwards (mirrored through x and z).
  add('cape', 'cape', [0, 24, -2], () => {
    const start = v.length;
    box(v, [10, 16, 1], [0, 0], [0, -8, -0.5], [64, 32]);
    for (let i = start; i < v.length; i += STRIDE) {
      v[i] = -v[i]!; v[i + 2] = -1 - v[i + 2]!; v[i + 5] = -v[i + 5]!; v[i + 7] = -v[i + 7]!;
    }
  });
  // The soft shadow under the feet: one quad, uv spans -1…1 for a radial falloff.
  add('floor', 'floor', [0, 0, 0], () => {
    for (const [x, z] of [[-1, -1], [1, -1], [1, 1], [-1, -1], [1, 1], [-1, 1]] as const) v.push(x * 9, 0.01, z * 5, x, z, 0, 1, 0, 0, 0, 0, 0);
  });
  return { data: new Float32Array(v), parts };
}

// ---- shaders ----
const VS = `attribute vec3 p;attribute vec2 t;attribute vec3 n;attribute vec4 r;
uniform mat4 mvp;uniform mat3 nm;varying vec2 vt;varying vec4 vr;varying float lit;
// Minecraft's entity lights, in view space, with its 0.4 ambient and 0.6 diffuse.
const vec3 L0=vec3(0.16169,0.80845,0.56592);const vec3 L1=vec3(-0.16169,0.80845,0.56592);
void main(){vec3 m=normalize(nm*n);lit=min(1.0,0.4+0.6*(max(dot(m,L0),0.0)+max(dot(m,L1),0.0)));vt=t;vr=r;gl_Position=mvp*vec4(p,1.0);}`;
// highp where the GPU has it: at mediump a UV is only ~1/16 texel exact, enough to round across an edge.
const FS = `#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif
uniform sampler2D tex;uniform int mode;varying vec2 vt;varying vec4 vr;varying float lit;
void main(){
if(mode==3){float d=length(vt);gl_FragColor=vec4(0.0,0.0,0.0,0.55*(1.0-smoothstep(0.0,1.0,d)));return;}
vec4 c=texture2D(tex,clamp(vt,vr.xy,vr.zw));
if(mode==0)c.a=1.0;else if(c.a<0.5)discard;
gl_FragColor=vec4(c.rgb*lit,1.0);}`;
const MODE = { base: 0, over: 1, cape: 2, floor: 3 } as const;

function compile(gl: WebGLRenderingContext): WebGLProgram | null {
  const prog = gl.createProgram();
  for (const [type, src] of [[gl.VERTEX_SHADER, VS], [gl.FRAGMENT_SHADER, FS]] as const) {
    const sh = gl.createShader(type);
    if (!sh || !prog) return null;
    gl.shaderSource(sh, src);
    gl.compileShader(sh);
    gl.attachShader(prog, sh);
    gl.deleteShader(sh);
  }
  if (!prog) return null;
  gl.bindAttribLocation(prog, 0, 'p');
  gl.bindAttribLocation(prog, 1, 't');
  gl.bindAttribLocation(prog, 2, 'n');
  gl.bindAttribLocation(prog, 3, 'r');
  gl.linkProgram(prog);
  return gl.getProgramParameter(prog, gl.LINK_STATUS) ? prog : null;
}

/** Field of view; small, so the model looks nearly flat like an inventory view. */
const FOV = (24 * Math.PI) / 180;

/**
 * Draws one player on a canvas. Call `draw` whenever something changed; it never loops on
 * its own. `ok` is false when WebGL isn't available (PlayerModel then shows a flat figure).
 */
export class SkinScene {
  ok = false;
  private gl: WebGLRenderingContext | null;
  private prog: WebGLProgram | null = null;
  private buf: WebGLBuffer | null = null;
  private parts: Record<string, Part> = {};
  private slim: boolean | null = null;
  private tex = new Map<'skin' | 'cape', { url: string; t: WebGLTexture | null; ready: boolean }>();
  private loc: { mvp: WebGLUniformLocation | null; nm: WebGLUniformLocation | null; mode: WebGLUniformLocation | null } = { mvp: null, nm: null, mode: null };

  /** `onReady` runs when a texture finished loading, so the caller can draw again. */
  constructor(private canvas: HTMLCanvasElement, private onReady: () => void) {
    this.gl = canvas.getContext('webgl', { antialias: true, alpha: true, premultipliedAlpha: true });
    this.init();
  }

  private init() {
    const gl = this.gl;
    this.prog = gl ? compile(gl) : null;
    if (!gl || !this.prog) return;
    this.loc = { mvp: gl.getUniformLocation(this.prog, 'mvp'), nm: gl.getUniformLocation(this.prog, 'nm'), mode: gl.getUniformLocation(this.prog, 'mode') };
    this.buf = gl.createBuffer();
    this.slim = null;
    for (const v of this.tex.values()) v.t = null;
    this.ok = true;
  }

  /** After a lost context comes back, everything on the GPU is rebuilt. */
  restore() {
    this.init();
    for (const [kind, v] of this.tex) this.texture(kind, v.url, true);
  }

  setSkin(url: string, slim: boolean) {
    if (!this.ok || !this.gl) return;
    if (slim !== this.slim) {
      const { data, parts } = buildPlayer(slim);
      this.parts = parts;
      this.slim = slim;
      this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this.buf);
      this.gl.bufferData(this.gl.ARRAY_BUFFER, data, this.gl.STATIC_DRAW);
    }
    this.texture('skin', url);
  }

  setCape(url: string | undefined) {
    if (!url) this.tex.delete('cape');
    else this.texture('cape', url);
  }

  private texture(kind: 'skin' | 'cape', url: string, force = false) {
    const gl = this.gl;
    const cur = this.tex.get(kind);
    if (!gl || (!force && cur?.url === url && cur.t)) return;
    if (cur?.t) gl.deleteTexture(cur.t);
    const entry = { url, t: gl.createTexture(), ready: false };
    this.tex.set(kind, entry);
    const img = new Image();
    img.onload = () => {
      if (this.tex.get(kind) !== entry || !entry.t) return;
      gl.bindTexture(gl.TEXTURE_2D, entry.t);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, img);
      for (const p of [gl.TEXTURE_MIN_FILTER, gl.TEXTURE_MAG_FILTER]) gl.texParameteri(gl.TEXTURE_2D, p, gl.NEAREST);
      for (const p of [gl.TEXTURE_WRAP_S, gl.TEXTURE_WRAP_T]) gl.texParameteri(gl.TEXTURE_2D, p, gl.CLAMP_TO_EDGE);
      entry.ready = true;
      this.onReady();
    };
    img.src = url;
  }

  /** Draws the player at `scale` CSS pixels per texel, centred in the canvas. */
  draw(pose: Pose, scale: number) {
    const gl = this.gl;
    if (!this.ok || !gl || !this.prog || gl.isContextLost()) return;
    const dpr = window.devicePixelRatio || 1;
    const w = this.canvas.clientWidth, h = this.canvas.clientHeight;
    if (!w || !h) return;
    if (this.canvas.width !== Math.round(w * dpr) || this.canvas.height !== Math.round(h * dpr)) {
      this.canvas.width = Math.round(w * dpr);
      this.canvas.height = Math.round(h * dpr);
    }
    gl.viewport(0, 0, this.canvas.width, this.canvas.height);
    gl.clearColor(0, 0, 0, 0);
    gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);
    gl.useProgram(this.prog);
    gl.bindBuffer(gl.ARRAY_BUFFER, this.buf);
    for (const [i, size, off] of [[0, 3, 0], [1, 2, 3], [2, 3, 5], [3, 4, 8]] as const) {
      gl.enableVertexAttribArray(i);
      gl.vertexAttribPointer(i, size, gl.FLOAT, false, STRIDE * 4, off * 4);
    }
    gl.enable(gl.DEPTH_TEST);
    gl.enable(gl.BLEND);
    gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);

    // The camera distance that makes one texel `scale` CSS pixels tall at the model's centre.
    const dist = h / (2 * scale * Math.tan(FOV / 2));
    const proj = perspective(FOV, w / h, Math.max(1, dist - 40), dist + 40);
    const world = chain(move([0, 0, -dist]), rot(0, pose.pitch), rot(1, pose.yaw), move([0, -16, 0]));
    const joints: Record<string, Mat> = {
      head: ident(),
      body: ident(),
      rarm: chain(rot(0, pose.swing), rot(2, -pose.sway)),
      larm: chain(rot(0, -pose.swing), rot(2, pose.sway)),
      rleg: rot(0, -pose.swing),
      lleg: rot(0, pose.swing),
      cape: rot(0, pose.cape),
      floor: ident(),
    };

    const skin = this.tex.get('skin'), cape = this.tex.get('cape');
    // The floor first, without writing depth, so the feet always sit on top of it.
    const order = ['floor', ...Object.keys(this.parts).filter((k) => k !== 'floor')];
    for (const name of order) {
      const part = this.parts[name]!;
      const t = part.layer === 'cape' ? cape : skin;
      if (part.layer !== 'floor' && !t?.ready) continue;
      const joint = joints[name.replace('+', '')]!;
      const model = chain(world, move(part.pivot), joint);
      gl.uniformMatrix4fv(this.loc.mvp, false, mul(proj, model));
      gl.uniformMatrix3fv(this.loc.nm, false, [model[0]!, model[1]!, model[2]!, model[4]!, model[5]!, model[6]!, model[8]!, model[9]!, model[10]!]);
      gl.uniform1i(this.loc.mode, MODE[part.layer]);
      gl.depthMask(part.layer !== 'floor');
      if (t?.t) gl.bindTexture(gl.TEXTURE_2D, t.t);
      gl.drawArrays(gl.TRIANGLES, part.first, part.count);
    }
    gl.depthMask(true);
  }

  /** Frees the GPU objects. */
  destroy() {
    const gl = this.gl;
    if (!gl) return;
    for (const v of this.tex.values()) if (v.t) gl.deleteTexture(v.t);
    this.tex.clear();
    if (this.buf) gl.deleteBuffer(this.buf);
    if (this.prog) gl.deleteProgram(this.prog);
    gl.getExtension('WEBGL_lose_context')?.loseContext();
    this.ok = false;
  }
}

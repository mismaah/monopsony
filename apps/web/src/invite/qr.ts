/**
 * A small QR encoder: byte mode, error-correction level M, versions 1–20.
 * That is far more than an invite URL needs, and it keeps the feature free of
 * a runtime dependency. Structure follows ISO/IEC 18004.
 */

/** Error-correction codewords per block, and block count, at level M. */
const ECC_M: ReadonlyArray<readonly [number, number]> = [
  [10, 1], [16, 1], [26, 1], [18, 2], [24, 2], [16, 4], [18, 4], [22, 4], [22, 5], [26, 5],
  [30, 5], [22, 8], [22, 9], [24, 9], [24, 10], [28, 10], [28, 11], [26, 13], [26, 14], [26, 16],
];

const MAX_VERSION = ECC_M.length;

/** Modules a version has left over once the function patterns are placed. */
function rawDataModules(ver: number): number {
  let n = (16 * ver + 128) * ver + 64;
  if (ver >= 2) {
    const align = Math.floor(ver / 7) + 2;
    n -= (25 * align - 10) * align - 55;
    if (ver >= 7) n -= 36; // the two version-information blocks
  }
  return n;
}

const totalCodewords = (ver: number) => Math.floor(rawDataModules(ver) / 8);

function dataCodewords(ver: number): number {
  const [ec, blocks] = ECC_M[ver - 1];
  return totalCodewords(ver) - ec * blocks;
}

/** Width of the byte-mode character-count field at this version. */
const countBits = (ver: number) => (ver < 10 ? 8 : 16);

function pickVersion(byteLength: number): number {
  for (let v = 1; v <= MAX_VERSION; v++) {
    if (4 + countBits(v) + byteLength * 8 <= dataCodewords(v) * 8) return v;
  }
  throw new Error("qr: payload too long");
}

// ---- data encoding -------------------------------------------------------------------

function encodeData(bytes: Uint8Array, ver: number): number[] {
  const bits: number[] = [];
  const push = (value: number, len: number) => {
    for (let i = len - 1; i >= 0; i--) bits.push((value >>> i) & 1);
  };
  push(0b0100, 4); // byte mode
  push(bytes.length, countBits(ver));
  for (const b of bytes) push(b, 8);

  const capacity = dataCodewords(ver) * 8;
  push(0, Math.min(4, capacity - bits.length)); // terminator
  while (bits.length % 8 !== 0) bits.push(0);

  const words: number[] = [];
  for (let i = 0; i < bits.length; i += 8) {
    let w = 0;
    for (let j = 0; j < 8; j++) w = (w << 1) | bits[i + j];
    words.push(w);
  }
  // Pad with the two alternating filler codewords the spec names.
  for (let pad = 0xec; words.length < dataCodewords(ver); pad ^= 0xec ^ 0x11) words.push(pad);
  return words;
}

// ---- Reed–Solomon --------------------------------------------------------------------

/** Multiply in GF(256) modulo the QR primitive polynomial x^8+x^4+x^3+x^2+1. */
function gfMul(a: number, b: number): number {
  let z = 0;
  for (let i = 7; i >= 0; i--) {
    z = (z << 1) ^ ((z >>> 7) * 0x11d);
    z ^= ((b >>> i) & 1) * a;
  }
  return z & 0xff;
}

/** Coefficients of the divisor polynomial, highest power first, monic term dropped. */
function rsGenerator(degree: number): number[] {
  const poly = new Array<number>(degree).fill(0);
  poly[degree - 1] = 1;
  let root = 1;
  for (let i = 0; i < degree; i++) {
    for (let j = 0; j < degree; j++) {
      poly[j] = gfMul(poly[j], root);
      if (j + 1 < degree) poly[j] ^= poly[j + 1];
    }
    root = gfMul(root, 2);
  }
  return poly;
}

function rsRemainder(data: readonly number[], generator: readonly number[]): number[] {
  const result = new Array<number>(generator.length).fill(0);
  for (const b of data) {
    const factor = b ^ (result.shift() as number);
    result.push(0);
    for (let i = 0; i < generator.length; i++) result[i] ^= gfMul(generator[i], factor);
  }
  return result;
}

/** Split into blocks, append each block's checkwords, and interleave. */
function codewords(bytes: Uint8Array, ver: number): number[] {
  const [ecLen, numBlocks] = ECC_M[ver - 1];
  const data = encodeData(bytes, ver);
  const shortLen = Math.floor(data.length / numBlocks);
  const numShort = numBlocks - (data.length % numBlocks);
  const generator = rsGenerator(ecLen);

  const blocks: number[][] = [];
  const checks: number[][] = [];
  for (let i = 0, off = 0; i < numBlocks; i++) {
    const len = shortLen + (i < numShort ? 0 : 1);
    const block = data.slice(off, off + len);
    off += len;
    blocks.push(block);
    checks.push(rsRemainder(block, generator));
  }

  const out: number[] = [];
  for (let i = 0; i <= shortLen; i++) for (const b of blocks) if (i < b.length) out.push(b[i]);
  for (let i = 0; i < ecLen; i++) for (const c of checks) out.push(c[i]);
  return out;
}

// ---- module placement ----------------------------------------------------------------

/** Centre coordinates shared by the alignment pattern rows and columns. */
function alignmentPositions(ver: number): number[] {
  if (ver === 1) return [];
  const count = Math.floor(ver / 7) + 2;
  const step = Math.ceil((ver * 4 + 4) / (count * 2 - 2)) * 2;
  const pos = [6];
  for (let p = ver * 4 + 10; pos.length < count; p -= step) pos.splice(1, 0, p);
  return pos;
}

interface Grid {
  size: number;
  /** true = dark. */
  modules: boolean[][];
  /** Function patterns and reserved areas, which data placement skips. */
  fixed: boolean[][];
}

const blank = (size: number) => Array.from({ length: size }, () => new Array<boolean>(size).fill(false));

function setFixed(g: Grid, x: number, y: number, dark: boolean) {
  if (x < 0 || y < 0 || x >= g.size || y >= g.size) return;
  g.modules[y][x] = dark;
  g.fixed[y][x] = true;
}

/**
 * Marks the format area — and, from version 7, the version area — as function
 * modules while leaving them light. Masks are scored before either is placed
 * (ISO/IEC 18004 §7.8), so these modules have to be reserved but blank.
 */
function reserveInfoAreas(g: Grid, ver: number) {
  for (let i = 0; i < 9; i++) {
    if (i === 6) continue; // the timing patterns cross here and stay as drawn
    setFixed(g, 8, i, false);
    setFixed(g, i, 8, false);
  }
  for (let i = 1; i <= 8; i++) {
    setFixed(g, 8, g.size - i, false);
    setFixed(g, g.size - i, 8, false);
  }
  if (ver >= 7) {
    for (const [x, y] of versionModules(g.size)) setFixed(g, x, y, false);
  }
}

/** The 2x18 version-information modules, paired as (x, y). */
function versionModules(size: number): [number, number][] {
  const cells: [number, number][] = [];
  for (let i = 0; i < 18; i++) {
    const a = size - 11 + (i % 3);
    const b = Math.floor(i / 3);
    cells.push([a, b], [b, a]);
  }
  return cells;
}

/** Writes the 18 version bits, which only versions 7 and up carry. */
function drawVersion(g: Grid, ver: number) {
  if (ver < 7) return;
  let rem = ver;
  for (let i = 0; i < 12; i++) rem = (rem << 1) ^ ((rem >>> 11) * 0x1f25);
  const bits = (ver << 12) | rem;
  const cells = versionModules(g.size);
  for (let i = 0; i < 18; i++) {
    const dark = ((bits >>> i) & 1) !== 0;
    setFixed(g, cells[i * 2][0], cells[i * 2][1], dark);
    setFixed(g, cells[i * 2 + 1][0], cells[i * 2 + 1][1], dark);
  }
}

/** Writes the 15 format bits (level M plus the mask) into both copies. */
function drawFormat(g: Grid, mask: number) {
  const data = (0b00 << 3) | mask; // level M
  let rem = data;
  for (let i = 0; i < 10; i++) rem = (rem << 1) ^ ((rem >>> 9) * 0x537);
  const bits = ((data << 10) | rem) ^ 0x5412;
  const bit = (i: number) => ((bits >>> i) & 1) !== 0;

  for (let i = 0; i <= 5; i++) setFixed(g, 8, i, bit(i));
  setFixed(g, 8, 7, bit(6));
  setFixed(g, 8, 8, bit(7));
  setFixed(g, 7, 8, bit(8));
  for (let i = 9; i < 15; i++) setFixed(g, 14 - i, 8, bit(i));

  for (let i = 0; i < 8; i++) setFixed(g, g.size - 1 - i, 8, bit(i));
  for (let i = 8; i < 15; i++) setFixed(g, 8, g.size - 15 + i, bit(i));
  setFixed(g, 8, g.size - 8, true); // the always-dark module
}

function drawFunctionPatterns(ver: number): Grid {
  const size = ver * 4 + 17;
  const g: Grid = { size, modules: blank(size), fixed: blank(size) };

  for (let i = 0; i < size; i++) {
    setFixed(g, 6, i, i % 2 === 0);
    setFixed(g, i, 6, i % 2 === 0);
  }

  // Finder patterns, drawn with their separators: dark rings at Chebyshev
  // distance 0–1 and 3, light at 2, and the separator at 4.
  for (const [cx, cy] of [[3, 3], [size - 4, 3], [3, size - 4]]) {
    for (let dy = -4; dy <= 4; dy++) {
      for (let dx = -4; dx <= 4; dx++) {
        const d = Math.max(Math.abs(dx), Math.abs(dy));
        setFixed(g, cx + dx, cy + dy, d !== 2 && d !== 4);
      }
    }
  }

  const pos = alignmentPositions(ver);
  const last = pos.length - 1;
  for (let i = 0; i <= last; i++) {
    for (let j = 0; j <= last; j++) {
      if ((i === 0 && j === 0) || (i === 0 && j === last) || (i === last && j === 0)) continue; // finder corners
      for (let dy = -2; dy <= 2; dy++) {
        for (let dx = -2; dx <= 2; dx++) {
          setFixed(g, pos[j] + dx, pos[i] + dy, Math.max(Math.abs(dx), Math.abs(dy)) !== 1);
        }
      }
    }
  }

  reserveInfoAreas(g, ver);
  return g;
}

/** Zigzag the interleaved codewords up and down the free columns. */
function drawCodewords(g: Grid, data: readonly number[]) {
  let i = 0;
  for (let right = g.size - 1; right >= 1; right -= 2) {
    if (right === 6) right = 5; // the vertical timing column is not a data column
    for (let vert = 0; vert < g.size; vert++) {
      for (let j = 0; j < 2; j++) {
        const x = right - j;
        const upward = ((right + 1) & 2) === 0;
        const y = upward ? g.size - 1 - vert : vert;
        if (!g.fixed[y][x] && i < data.length * 8) {
          g.modules[y][x] = ((data[i >>> 3] >>> (7 - (i & 7))) & 1) !== 0;
          i++;
        }
      }
    }
  }
}

function maskBit(mask: number, x: number, y: number): boolean {
  switch (mask) {
    case 0: return (x + y) % 2 === 0;
    case 1: return y % 2 === 0;
    case 2: return x % 3 === 0;
    case 3: return (x + y) % 3 === 0;
    case 4: return (Math.floor(x / 3) + Math.floor(y / 2)) % 2 === 0;
    case 5: return ((x * y) % 2) + ((x * y) % 3) === 0;
    case 6: return (((x * y) % 2) + ((x * y) % 3)) % 2 === 0;
    default: return (((x + y) % 2) + ((x * y) % 3)) % 2 === 0;
  }
}

/** XOR the mask over every module that is not a function pattern. */
function applyMask(g: Grid, mask: number) {
  for (let y = 0; y < g.size; y++) {
    for (let x = 0; x < g.size; x++) {
      if (!g.fixed[y][x] && maskBit(mask, x, y)) g.modules[y][x] = !g.modules[y][x];
    }
  }
}

/** The 1:1:3:1:1 core of a finder pattern, dark first. */
const FINDER_CORE = [true, false, true, true, true, false, true];

/**
 * Penalty rule 3 along one row or column: the finder-like core counts when a
 * four-module light area runs alongside it, and falling off the edge of the
 * symbol counts as light. Overlapping cores are taken the way the reference
 * encoders take them — a counted core consumes its seven modules, an
 * uncounted one only its leading four.
 */
function finderPenalty(line: readonly boolean[]): number {
  const size = line.length;
  const clear = (from: number, to: number) => !line.slice(Math.max(from, 0), Math.min(to, size)).some(Boolean);
  let score = 0;
  let i = 0;
  while (i + 7 <= size) {
    if (!FINDER_CORE.every((want, k) => line[i + k] === want)) {
      i++;
      continue;
    }
    if (i === 0 || i === size - 7 || clear(i - 4, i) || clear(i + 7, i + 11)) {
      score += 40;
      i += 7;
    } else {
      i += 4;
    }
  }
  return score;
}

/** The spec's four penalty rules; the mask with the lowest total wins. */
function penalty(g: Grid): number {
  const { size, modules } = g;
  let score = 0;

  const runScore = (run: number) => (run >= 5 ? 3 + (run - 5) : 0);
  for (let i = 0; i < size; i++) {
    let rowRun = 1;
    let colRun = 1;
    for (let j = 1; j < size; j++) {
      if (modules[i][j] === modules[i][j - 1]) rowRun++;
      else {
        score += runScore(rowRun);
        rowRun = 1;
      }
      if (modules[j][i] === modules[j - 1][i]) colRun++;
      else {
        score += runScore(colRun);
        colRun = 1;
      }
    }
    score += runScore(rowRun) + runScore(colRun);
  }

  for (let y = 0; y + 1 < size; y++) {
    for (let x = 0; x + 1 < size; x++) {
      const v = modules[y][x];
      if (v === modules[y][x + 1] && v === modules[y + 1][x] && v === modules[y + 1][x + 1]) score += 3;
    }
  }

  for (let i = 0; i < size; i++) {
    score += finderPenalty(modules[i]);
    score += finderPenalty(modules.map((row) => row[i]));
  }

  let dark = 0;
  for (const row of modules) for (const m of row) if (m) dark++;
  const total = size * size;
  score += Math.floor(Math.abs(dark * 20 - total * 10) / total) * 10;
  return score;
}

/**
 * Encodes text as a QR symbol and returns its modules, row-major, true = dark.
 * The quiet zone is the caller's business. Throws if the text will not fit.
 */
export function qrMatrix(text: string): boolean[][] {
  const bytes = new TextEncoder().encode(text);
  const ver = pickVersion(bytes.length);
  const data = codewords(bytes, ver);

  let best: Grid | null = null;
  let bestMask = 0;
  let bestScore = Infinity;
  for (let mask = 0; mask < 8; mask++) {
    const g = drawFunctionPatterns(ver);
    drawCodewords(g, data);
    applyMask(g, mask);
    const score = penalty(g);
    if (score < bestScore) {
      bestScore = score;
      bestMask = mask;
      best = g;
    }
  }

  const symbol = best as Grid;
  drawFormat(symbol, bestMask);
  drawVersion(symbol, ver);
  return symbol.modules;
}

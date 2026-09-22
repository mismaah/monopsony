// Board geometry. Space 0 (Go) is the bottom-right corner; play proceeds
// counter-clockwise: along the bottom (right→left), up the left side, along
// the top (left→right), down the right side. Y is up; +Z is toward the camera.

export const TILE = 1; // width of an edge tile
export const DEPTH = 1.6; // depth of an edge tile / size of a corner
export const SIDE = 9 * TILE + 2 * DEPTH; // 12.2
export const HALF = SIDE / 2;

export type Side = 0 | 1 | 2 | 3; // bottom, left, top, right

export interface Tile {
  index: number;
  side: Side;
  corner: boolean;
  /** Centre of the tile on the board plane. */
  x: number;
  z: number;
  /** Rotation (radians about Y) so the tile's "inner" edge faces the centre. */
  rot: number;
  w: number; // extent along the side
  d: number; // extent toward the centre
}

export function tileFor(i: number): Tile {
  const side = Math.floor(i / 10) as Side;
  const k = i % 10;
  const corner = k === 0;
  const w = corner ? DEPTH : TILE;
  const d = DEPTH;
  // Distance along the side from the starting corner's centre to the tile centre.
  const along = corner ? 0 : DEPTH / 2 + TILE / 2 + (k - 1) * TILE;
  const c = HALF - DEPTH / 2; // corner centre offset from origin
  switch (side) {
    case 0:
      return { index: i, side, corner, x: c - along, z: c, rot: 0, w, d };
    case 1:
      return { index: i, side, corner, x: -c, z: c - along, rot: -Math.PI / 2, w, d };
    case 2:
      return { index: i, side, corner, x: -c + along, z: -c, rot: Math.PI, w, d };
    default:
      return { index: i, side, corner, x: c, z: -c + along, rot: Math.PI / 2, w, d };
  }
}

export const TILES: Tile[] = Array.from({ length: 40 }, (_, i) => tileFor(i));

/**
 * Where a token stands on a tile. Several tokens on one tile spread out in a
 * small grid so they never overlap; seat is the index among those present.
 */
export function tokenSpot(i: number, seat: number, count: number): [number, number, number] {
  const t = TILES[i];
  const cols = Math.min(count, 2);
  const row = Math.floor(seat / cols);
  const col = seat % cols;
  const spread = 0.28;
  // local offsets: u along the side, v toward the centre
  const u = (col - (cols - 1) / 2) * spread;
  const v = (row - (Math.ceil(count / cols) - 1) / 2) * spread + 0.15;
  const [x, z] = rotate(u, v, t.rot);
  return [t.x + x, 0.12, t.z + z];
}

/** Local (u = along side, v = toward centre) → world offset for a tile rotation. */
export function rotate(u: number, v: number, rot: number): [number, number] {
  // For side 0 (rot 0): along = -x direction of play doesn't matter for spread; v (toward centre) = -z.
  const cos = Math.cos(rot);
  const sin = Math.sin(rot);
  const lx = u;
  const lz = -v;
  return [lx * cos - lz * sin, lx * sin + lz * cos];
}

/** Position of the coloured band / buildings strip on a street tile. */
export function bandSpot(i: number): { pos: [number, number, number]; rot: number; len: number } {
  const t = TILES[i];
  const [x, z] = rotate(0, t.d / 2 - 0.18, t.rot);
  return { pos: [t.x + x, 0.06, t.z + z], rot: t.rot, len: t.w };
}

/** Owner marker on the outer edge of a tile. */
export function ownerSpot(i: number): [number, number, number] {
  const t = TILES[i];
  const [x, z] = rotate(0, -t.d / 2 + 0.16, t.rot);
  return [t.x + x, 0.07, t.z + z];
}

/** House n (0..3) of a street, or the hotel, sits on the band. */
export function buildingSpot(i: number, n: number, hotel: boolean): [number, number, number] {
  const t = TILES[i];
  const u = hotel ? 0 : (n - 1.5) * 0.22;
  const [x, z] = rotate(u, t.d / 2 - 0.18, t.rot);
  return [t.x + x, 0.16, t.z + z];
}

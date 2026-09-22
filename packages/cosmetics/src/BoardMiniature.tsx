import { TILES, HALF, DEPTH, bandSpot, buildingSpot } from "./layout";
import type { BoardSkin } from "./skins";

/**
 * A board drawn from a skin alone: the same slabs, tiles and bands as the
 * table, without a game config or state behind it. Used by the previews so
 * a palette can be judged before anyone equips it. The colour groups and
 * the scatter of buildings/mortgages are fixed set dressing that shows off
 * every key in the palette.
 */
const SIDE_TABLE = HALF * 2 + 6;
const CENTRE = HALF * 2 - DEPTH * 2 - 0.1;

const GROUPS: [number[], string][] = [
  [[1, 3], "#8b5a2b"],
  [[6, 8, 9], "#7dd3fc"],
  [[11, 13, 14], "#d946ef"],
  [[16, 18, 19], "#f97316"],
  [[21, 23, 24], "#ef4444"],
  [[26, 27, 29], "#facc15"],
  [[31, 32, 34], "#22c55e"],
  [[37, 39], "#2563eb"],
];
const BANDS: Record<number, string> = {};
for (const [tiles, color] of GROUPS) for (const t of tiles) BANDS[t] = color;

const HOUSES: Record<number, number> = { 6: 2, 8: 3, 21: 4 };
const HOTELS = [31, 32];
const MORTGAGED = [13];

export function BoardMiniature({ skin }: { skin: BoardSkin }) {
  return (
    <group>
      <mesh position={[0, -0.3, 0]}>
        <boxGeometry args={[SIDE_TABLE, 0.4, SIDE_TABLE]} />
        <meshStandardMaterial color={skin.table} roughness={0.95} />
      </mesh>
      <mesh position={[0, -0.05, 0]}>
        <boxGeometry args={[HALF * 2 + 0.2, 0.1, HALF * 2 + 0.2]} />
        <meshStandardMaterial color={skin.tileEdge} roughness={0.8} />
      </mesh>
      <mesh position={[0, 0.005, 0]} rotation={[-Math.PI / 2, 0, 0]}>
        <planeGeometry args={[CENTRE, CENTRE]} />
        <meshStandardMaterial color={skin.centre} roughness={0.9} />
      </mesh>
      {TILES.map((t) => {
        const band = BANDS[t.index] ? bandSpot(t.index) : null;
        const houses = HOUSES[t.index] ?? 0;
        return (
          <group key={t.index}>
            <mesh position={[t.x, 0, t.z]} rotation={[0, t.rot, 0]}>
              <boxGeometry args={[t.w - 0.04, 0.1, t.d - 0.04]} />
              <meshStandardMaterial color={MORTGAGED.includes(t.index) ? skin.mortgageTint : skin.tile} roughness={0.85} />
            </mesh>
            {band && (
              <mesh position={band.pos} rotation={[0, band.rot, 0]}>
                <boxGeometry args={[band.len - 0.08, 0.03, 0.3]} />
                <meshStandardMaterial color={BANDS[t.index]} roughness={0.7} />
              </mesh>
            )}
            {HOTELS.includes(t.index) && (
              <mesh position={buildingSpot(t.index, 0, true)} rotation={[0, t.rot, 0]}>
                <boxGeometry args={[0.5, 0.2, 0.22]} />
                <meshStandardMaterial color={skin.hotel} />
              </mesh>
            )}
            {Array.from({ length: houses }).map((_, n) => (
              <mesh key={n} position={buildingSpot(t.index, n, false)} rotation={[0, t.rot, 0]}>
                <boxGeometry args={[0.16, 0.14, 0.16]} />
                <meshStandardMaterial color={skin.house} />
              </mesh>
            ))}
          </group>
        );
      })}
    </group>
  );
}

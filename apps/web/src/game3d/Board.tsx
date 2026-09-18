import { Suspense, useMemo } from "react";
import { Text, useTexture } from "@react-three/drei";
import * as THREE from "three";
import type { ThreeEvent } from "@react-three/fiber";
import type { Config, State } from "@monopsony/protocol";
import { TILES, HALF, DEPTH, bandSpot, ownerSpot, buildingSpot, rotate } from "./layout";
import type { BoardSkin } from "./skins";
import { seatColors } from "@/lib/ui";

interface Props {
  config: Config;
  state: State;
  skin: BoardSkin;
  highlight: number | null;
  selected: number | null;
  onSelect: (i: number | null) => void;
}

const FONT_URL = undefined; // drei's default (Roboto via CDN); swap for a bundled font in the asset pipeline

function shortName(name: string) {
  return name
    .replace(" Avenue", " Ave")
    .replace(" Railroad", " RR")
    .replace("Community Chest", "Chest")
    .replace(" Terrace", " Terr")
    .replace(" Square", " Sq");
}

const CENTRE = HALF * 2 - DEPTH * 2 - 0.1; // side of the centre panel
const EMBLEM = CENTRE * 0.62;

/** The brand emblem printed in the middle of the board (public/brand/board-centre.png). */
function CentreEmblem() {
  const map = useTexture("/brand/board-centre.png");
  map.colorSpace = THREE.SRGBColorSpace;
  map.anisotropy = 8;
  return (
    <mesh position={[0, 0.012, 0]} rotation={[-Math.PI / 2, 0, Math.PI / 4]}>
      <planeGeometry args={[EMBLEM, EMBLEM]} />
      <meshStandardMaterial map={map} transparent roughness={0.85} />
    </mesh>
  );
}

export function Board({ config, state, skin, highlight, selected, onSelect }: Props) {
  const seatIndex = useMemo(() => {
    const m: Record<string, number> = {};
    state.players.forEach((p, i) => (m[p.id] = i));
    return m;
  }, [state.players]);

  return (
    <group>
      {/* table + board base */}
      <mesh position={[0, -0.3, 0]} receiveShadow>
        <boxGeometry args={[SIDE_TABLE, 0.4, SIDE_TABLE]} />
        <meshStandardMaterial color={skin.table} roughness={0.95} />
      </mesh>
      <mesh position={[0, -0.05, 0]} receiveShadow>
        <boxGeometry args={[HALF * 2 + 0.2, 0.1, HALF * 2 + 0.2]} />
        <meshStandardMaterial color={skin.tileEdge} roughness={0.8} />
      </mesh>
      {/* centre panel */}
      <mesh position={[0, 0.005, 0]} rotation={[-Math.PI / 2, 0, 0]} receiveShadow>
        <planeGeometry args={[CENTRE, CENTRE]} />
        <meshStandardMaterial color={skin.centre} roughness={0.9} />
      </mesh>
      <Suspense fallback={null}>
        <CentreEmblem />
      </Suspense>
      {/* the board's name sits under the emblem, along the same diagonal, so a retheme still shows */}
      <Text
        position={[EMBLEM * 0.42, 0.02, EMBLEM * 0.42]}
        rotation={[-Math.PI / 2, 0, Math.PI / 4]}
        fontSize={0.42}
        color={skin.text}
        anchorX="center"
        anchorY="middle"
        font={FONT_URL}
        fillOpacity={0.6}
        letterSpacing={0.15}
      >
        {config.name.toUpperCase()}
      </Text>

      {TILES.map((t) => {
        const def = config.spaces[t.index];
        const st = state.spaces[t.index];
        const owner = st.ownerId ? seatIndex[st.ownerId] : undefined;
        const isHi = highlight === t.index;
        const isSel = selected === t.index;
        const tileColor = st.mortgaged ? skin.mortgageTint : isHi ? "#fde68a" : isSel ? "#bfdbfe" : skin.tile;
        const band = def.type === "street" ? bandSpot(t.index) : null;
        const labelOff = rotate(0, -0.15, t.rot);
        return (
          <group key={t.index}>
            <mesh
              position={[t.x, 0.0, t.z]}
              rotation={[0, t.rot, 0]}
              receiveShadow
              onClick={(e: ThreeEvent<MouseEvent>) => {
                e.stopPropagation();
                onSelect(selected === t.index ? null : t.index);
              }}
              onPointerOver={() => (document.body.style.cursor = "pointer")}
              onPointerOut={() => (document.body.style.cursor = "default")}
            >
              <boxGeometry args={[t.w - 0.04, 0.1, t.d - 0.04]} />
              <meshStandardMaterial color={tileColor} roughness={0.85} />
            </mesh>
            {band && (
              <mesh position={band.pos} rotation={[0, band.rot, 0]}>
                <boxGeometry args={[band.len - 0.08, 0.03, 0.3]} />
                <meshStandardMaterial color={def.color || "#888"} roughness={0.7} />
              </mesh>
            )}
            <Text
              position={[t.x + labelOff[0], 0.06, t.z + labelOff[1]]}
              rotation={[-Math.PI / 2, 0, -t.rot + (t.corner ? Math.PI / 4 : 0)]}
              fontSize={t.corner ? 0.2 : 0.13}
              maxWidth={t.corner ? 1.3 : 0.9}
              textAlign="center"
              color={skin.text}
              anchorX="center"
              anchorY="middle"
              font={FONT_URL}
            >
              {shortName(def.name).toUpperCase()}
              {def.price ? `\n$${def.price}` : ""}
            </Text>
            {owner !== undefined && (
              <mesh position={ownerSpot(t.index)} rotation={[-Math.PI / 2, 0, 0]}>
                <circleGeometry args={[0.11, 24]} />
                <meshStandardMaterial color={seatColors[owner % seatColors.length]} emissive={seatColors[owner % seatColors.length]} emissiveIntensity={0.3} />
              </mesh>
            )}
            {st.houses > 0 &&
              (st.houses === 5 ? (
                <mesh position={buildingSpot(t.index, 0, true)} rotation={[0, t.rot, 0]} castShadow>
                  <boxGeometry args={[0.5, 0.2, 0.22]} />
                  <meshStandardMaterial color={skin.hotel} />
                </mesh>
              ) : (
                Array.from({ length: st.houses }).map((_, n) => (
                  <mesh key={n} position={buildingSpot(t.index, n, false)} rotation={[0, t.rot, 0]} castShadow>
                    <boxGeometry args={[0.16, 0.14, 0.16]} />
                    <meshStandardMaterial color={skin.house} />
                  </mesh>
                ))
              ))}
          </group>
        );
      })}
    </group>
  );
}

const SIDE_TABLE = HALF * 2 + 6;

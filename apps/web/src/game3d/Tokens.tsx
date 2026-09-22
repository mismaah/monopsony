import { useEffect, useMemo, useRef } from "react";
import * as THREE from "three";
import gsap from "gsap";
import type { State, SeatInfo } from "@monopsony/protocol";
import { tokenSpot, tokenSkinFor, TokenMesh, type TokenSkin } from "@monopsony/cosmetics";
import { useCosmetics } from "@/store/cosmetics";
import { seatColors } from "@/lib/ui";
import { T } from "@/anim/timing";

interface Props {
  state: State;
  seats: SeatInfo[];
  positions: Record<string, number>;
  activeId: string | null;
}

/** Tokens hop from tile to tile as the animation queue advances positions. */
export function Tokens({ state, seats, positions, activeId }: Props) {
  const manifests = useCosmetics((s) => s.manifests);
  // Group tokens per tile so several on one space spread out.
  const occupancy = useMemo(() => {
    const byTile: Record<number, string[]> = {};
    state.players.forEach((p) => {
      if (p.bankrupt) return;
      const pos = positions[p.id] ?? p.position;
      (byTile[pos] ??= []).push(p.id);
    });
    return byTile;
  }, [state.players, positions]);

  return (
    <group>
      {state.players.map((p, i) => {
        if (p.bankrupt) return null;
        const pos = positions[p.id] ?? p.position;
        const group = occupancy[pos] ?? [p.id];
        const target = tokenSpot(pos, group.indexOf(p.id), group.length);
        const loadout = seats.find((s) => s.playerId === p.id)?.loadout;
        return (
          <Token
            key={p.id}
            target={target}
            skin={tokenSkinFor(manifests, loadout, i)}
            color={seatColors[i % seatColors.length]}
            active={activeId === p.id}
            inJail={p.inJail}
          />
        );
      })}
    </group>
  );
}

function Token({ target, skin, color, active, inJail }: { target: [number, number, number]; skin: TokenSkin; color: string; active: boolean; inJail: boolean }) {
  const ref = useRef<THREE.Group>(null);
  const first = useRef(true);

  useEffect(() => {
    const g = ref.current;
    if (!g) return;
    const [x, y, z] = target;
    if (first.current) {
      g.position.set(x, y, z);
      first.current = false;
      return;
    }
    const dist = Math.hypot(g.position.x - x, g.position.z - z);
    const far = dist > 2.2; // teleport (card / jail): a higher arc
    const dur = (far ? T.tokenTeleport : T.tokenStep) / 1000;
    gsap.killTweensOf(g.position);
    const tl = gsap.timeline();
    tl.to(g.position, { x, z, duration: dur, ease: far ? "power2.inOut" : "power1.inOut" }, 0);
    tl.to(g.position, { y: y + (far ? 1.2 : 0.35), duration: dur / 2, ease: "power2.out" }, 0);
    tl.to(g.position, { y, duration: dur / 2, ease: "bounce.out" }, dur / 2);
    return () => {
      tl.kill();
    };
  }, [target[0], target[1], target[2]]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <group ref={ref}>
      <TokenMesh skin={skin} color={color} active={active} />
      {inJail && (
        <mesh position={[0, 0.3, 0]}>
          <torusGeometry args={[0.32, 0.02, 8, 32]} />
          <meshStandardMaterial color="#9ca3af" metalness={0.8} roughness={0.3} />
        </mesh>
      )}
      {active && (
        <mesh position={[0, 0.02, 0]} rotation={[-Math.PI / 2, 0, 0]}>
          <ringGeometry args={[0.28, 0.34, 32]} />
          <meshBasicMaterial color={color} transparent opacity={0.8} />
        </mesh>
      )}
    </group>
  );
}

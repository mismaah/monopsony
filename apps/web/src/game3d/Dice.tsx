import { useEffect, useMemo, useRef } from "react";
import * as THREE from "three";
import gsap from "gsap";
import { dieMaterials, UP_ROTATION, type DiceSkin } from "@monopsony/cosmetics";
import { T } from "@/anim/timing";

interface Props {
  faces: [number, number];
  rollId: number;
  skin: DiceSkin;
}

export function Dice({ faces, rollId, skin }: Props) {
  const materials = useMemo(() => dieMaterials(skin), [skin]);
  return (
    <group position={[0, 0, 1.6]}>
      <Die materials={materials} face={faces[0]} rollId={rollId} offset={[-0.55, 0, 0]} seed={1} />
      <Die materials={materials} face={faces[1]} rollId={rollId} offset={[0.55, 0, 0.3]} seed={2} />
    </group>
  );
}

function Die({ materials, face, rollId, offset, seed }: { materials: THREE.Material[]; face: number; rollId: number; offset: [number, number, number]; seed: number }) {
  const ref = useRef<THREE.Mesh>(null);
  const first = useRef(true);
  // Read through refs so the tumble effect only re-runs on a new roll, not
  // whenever the parent re-renders with a fresh offset array.
  const faceRef = useRef(face);
  faceRef.current = face;
  const offsetRef = useRef(offset);
  offsetRef.current = offset;

  useEffect(() => {
    const m = ref.current;
    if (!m) return;
    const face = faceRef.current;
    const offset = offsetRef.current;
    const [rx, ry, rz] = UP_ROTATION[face] ?? [0, 0, 0];
    if (first.current || rollId === 0) {
      m.rotation.set(rx, ry, rz);
      m.position.set(offset[0], 0.3, offset[2]);
      first.current = false;
      return;
    }
    // Tumble: throw up, spin a few random turns, land on the exact face.
    const dur = T.diceRoll / 1000;
    const spins = 2 + seed;
    gsap.killTweensOf(m.rotation);
    gsap.killTweensOf(m.position);
    const tl = gsap.timeline();
    tl.fromTo(m.position, { x: offset[0] + (seed === 1 ? -1.5 : 1.5), y: 1.6, z: offset[2] - 1 }, { x: offset[0], y: 0.3, z: offset[2], duration: dur, ease: "bounce.out" }, 0);
    tl.to(m.rotation, { x: rx + Math.PI * 2 * spins, y: ry + Math.PI * 2 * (spins - 1), z: rz + Math.PI * 2 * spins, duration: dur, ease: "power2.out" }, 0);
    tl.set(m.rotation, { x: rx, y: ry, z: rz });
    return () => {
      tl.kill();
    };
  }, [rollId, seed]);

  return (
    <mesh ref={ref} material={materials} castShadow position={[offset[0], 0.3, offset[2]]}>
      <boxGeometry args={[0.5, 0.5, 0.5]} />
    </mesh>
  );
}

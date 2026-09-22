import React, { Suspense, useEffect, useMemo, useRef } from "react";
import { Canvas, useThree } from "@react-three/fiber";
import { OrbitControls, ContactShadows, Environment, AdaptiveDpr } from "@react-three/drei";
import * as THREE from "three";
import gsap from "gsap";
import { useGame } from "@/store/game";
import { useAuth } from "@/store/auth";
import { Board } from "./Board";
import { Tokens } from "./Tokens";
import { Dice } from "./Dice";
import { resolveBoard, resolveDice, TILES } from "@monopsony/cosmetics";
import { useCosmetics } from "@/store/cosmetics";

/** Camera follows the active token when "follow" is on, otherwise free orbit. */
function CameraRig({ follow, focusSpace }: { follow: boolean; focusSpace: number | null }) {
  const controls = useRef<React.ElementRef<typeof OrbitControls>>(null);
  const { camera } = useThree();
  useEffect(() => {
    const c = controls.current;
    if (!c) return;
    const target = follow && focusSpace !== null ? new THREE.Vector3(TILES[focusSpace].x * 0.3, 0, TILES[focusSpace].z * 0.3) : new THREE.Vector3(0, 0, 0);
    gsap.to(c.target, { x: target.x, y: target.y, z: target.z, duration: 0.8, ease: "power2.inOut", onUpdate: () => c.update() });
  }, [follow, focusSpace]);
  useEffect(() => {
    camera.position.set(0, 15.5, 12.5);
  }, [camera]);
  return <OrbitControls ref={controls} enablePan={false} minDistance={8} maxDistance={28} maxPolarAngle={Math.PI / 2.2} minPolarAngle={0.35} />;
}

export function Scene({ follow }: { follow: boolean }) {
  const { config, state, seats, positions, dice, highlight, selectedSpace, select, waitingOn } = useGame();
  const me = useAuth((s) => s.user);
  const manifests = useCosmetics((s) => s.manifests);
  const myLoadout = seats.find((s) => s.playerId === me?.id)?.loadout;
  const board = resolveBoard(manifests, myLoadout?.board);
  // Stable identity: Dice memoizes its materials on the skin object, and the
  // scene re-renders on every token move.
  const diceSkin = useMemo(() => resolveDice(manifests, myLoadout?.dice), [manifests, myLoadout?.dice]);
  // An Update (state only) can arrive a frame before the Snapshot that
  // carries the config, so this early return must come after every hook.
  if (!config || !state) return null;
  const activeId = state.turn.phase === "game_over" ? null : state.players[state.turn.playerIdx]?.id ?? null;
  const focusId = waitingOn[0] ?? activeId;
  const focusSpace = focusId ? positions[focusId] ?? null : null;

  return (
    <Canvas shadows dpr={[1, 2]} camera={{ fov: 42, near: 0.1, far: 100 }} onPointerMissed={() => select(null)}>
      <color attach="background" args={["#0b1220"]} />
      <ambientLight intensity={0.5} />
      <directionalLight position={[8, 14, 6]} intensity={1.6} castShadow shadow-mapSize={[2048, 2048]} shadow-camera-left={-10} shadow-camera-right={10} shadow-camera-top={10} shadow-camera-bottom={-10} />
      <Suspense fallback={null}>
        <Environment preset="city" />
        <Board config={config} state={state} skin={board} highlight={highlight} selected={selectedSpace} onSelect={select} />
        <Tokens state={state} seats={seats} positions={positions} activeId={activeId} />
        <Dice faces={dice.faces} rollId={dice.rollId} skin={diceSkin} />
        <ContactShadows position={[0, 0.01, 0]} opacity={0.5} scale={20} blur={1.5} far={2} />
      </Suspense>
      <CameraRig follow={follow} focusSpace={focusSpace} />
      <AdaptiveDpr pixelated />
    </Canvas>
  );
}

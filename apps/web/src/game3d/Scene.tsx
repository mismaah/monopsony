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
import { resolveBoard, resolveDice, TILES, HALF, DEPTH } from "@monopsony/cosmetics";
import { useCosmetics } from "@/store/cosmetics";
import { useIsTouch } from "@/lib/responsive";

const FOV = 42;
/** The camera's resting direction; its length is the desktop viewing distance. */
const EYE = new THREE.Vector3(0, 15.5, 12.5);
/** Centre offset of a corner tile — the furthest the follow camera ever pans. */
const CORNER = HALF - DEPTH / 2;
/**
 * Half-extents the view has to cover, in world units: the board itself plus
 * the spread perspective adds to the near edge, which sits closer to the
 * camera than the point we focus on.
 */
const SPAN_X = HALF * 1.08 + 0.3;
const SPAN_Y = 6.4;

/**
 * Where to put the camera, and how far the follow camera may pan, for a
 * viewport of this shape.
 *
 * A desktop window is roomy in both directions and keeps the house framing.
 * A portrait phone is not wide enough for the board at that distance, so the
 * camera pulls back and pans less (a full pan would shove the board's left
 * edge off screen). A short window — a phone held sideways — leaves the board
 * room for the HUD strips above and below it.
 */
export function frameFor(aspect: number, heightPx: number): { dist: number; pan: number; lift: number } {
  const t = Math.tan((FOV * Math.PI) / 360);
  const narrow = aspect < 1.1;
  const short = heightPx > 0 && heightPx < 520;
  const pan = narrow ? 0.12 : 0.3;
  const forWidth = (SPAN_X + pan * CORNER) / (t * Math.max(aspect, 0.2));
  const forHeight = short ? SPAN_Y / (t * 0.62) : 0;
  // On a phone the bottom strip is the taller of the two, so the gap the
  // board sits in is above the middle of the screen; drop the pivot to
  // centre the board in that gap instead of in the canvas.
  return { dist: Math.max(EYE.length(), forWidth, forHeight), pan, lift: narrow ? -3 : short ? -2.5 : 0 };
}

/** Camera follows the active token when "follow" is on, otherwise free orbit. */
function CameraRig({ follow, focusSpace }: { follow: boolean; focusSpace: number | null }) {
  const controls = useRef<React.ElementRef<typeof OrbitControls>>(null);
  const { camera, size } = useThree();
  const { dist, pan, lift } = frameFor(size.width / Math.max(1, size.height), size.height);

  useEffect(() => {
    const c = controls.current;
    if (!c) return;
    const target = follow && focusSpace !== null ? new THREE.Vector3(TILES[focusSpace].x * pan, lift, TILES[focusSpace].z * pan) : new THREE.Vector3(0, lift, 0);
    gsap.to(c.target, { x: target.x, y: target.y, z: target.z, duration: 0.8, ease: "power2.inOut", onUpdate: () => c.update() });
  }, [follow, focusSpace, pan, lift]);

  // Re-frame whenever the window's shape changes (rotation, URL bar, resize),
  // keeping the angle the player is looking from and only changing the range.
  const framed = useRef(false);
  useEffect(() => {
    // First pass sets the house angle; later ones keep whatever angle the
    // player has orbited to and only change how far back the camera sits.
    const dir = framed.current && camera.position.length() > 0.01 ? camera.position.clone().normalize() : EYE.clone().normalize();
    framed.current = true;
    camera.position.copy(dir.multiplyScalar(dist));
    controls.current?.update();
  }, [camera, dist]);

  return (
    <OrbitControls
      ref={controls}
      enablePan={false}
      // One finger orbits, two fingers zoom; panning stays off so a stray
      // second touch can never shove the board out of frame.
      touches={{ ONE: THREE.TOUCH.ROTATE, TWO: THREE.TOUCH.DOLLY_ROTATE }}
      minDistance={8}
      maxDistance={Math.max(28, dist * 1.3)}
      maxPolarAngle={Math.PI / 2.2}
      minPolarAngle={0.35}
    />
  );
}

export function Scene({ follow }: { follow: boolean }) {
  const { config, state, seats, positions, dice, highlight, selectedSpace, select, waitingOn } = useGame();
  const me = useAuth((s) => s.user);
  const manifests = useCosmetics((s) => s.manifests);
  const touch = useIsTouch();
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
    <Canvas
      shadows
      // Phones pay for every pixel: cap the ratio and shrink the shadow map
      // rather than dropping frames mid-roll.
      dpr={[1, touch ? 1.5 : 2]}
      camera={{ fov: FOV, near: 0.1, far: 120 }}
      onPointerMissed={() => select(null)}
    >
      <color attach="background" args={["#0b1220"]} />
      <ambientLight intensity={0.5} />
      <directionalLight
        position={[8, 14, 6]}
        intensity={1.6}
        castShadow
        shadow-mapSize={touch ? [1024, 1024] : [2048, 2048]}
        shadow-camera-left={-10}
        shadow-camera-right={10}
        shadow-camera-top={10}
        shadow-camera-bottom={-10}
      />
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

import { Canvas, useFrame } from "@react-three/fiber";
import { PerspectiveCamera, View } from "@react-three/drei";
import { useEffect, useMemo, useRef, type CSSProperties, type ReactNode } from "react";
import * as THREE from "three";
import { BOARDS, resolveBoard, resolveCards, resolveDice, resolveToken, type Manifest } from "./skins";
import { TokenMesh } from "./TokenMesh";
import { dieMaterials, UP_ROTATION } from "./die";
import { BoardMiniature } from "./BoardMiniature";

/**
 * One WebGL context for every preview on a page. Browsers keep only around
 * sixteen contexts alive, so a catalog grid of per-card canvases starts
 * losing its oldest ones. Instead each CosmeticPreview is a plain div and
 * this canvas, fixed over the viewport and transparent, draws into their
 * rectangles (drei's View). Wrap the page (or the part of it that shows
 * previews) once; previews rendered outside a stage stay empty.
 */
export function PreviewStage({ children, className }: { children: ReactNode; className?: string }) {
  const ref = useRef<HTMLDivElement>(null!);
  return (
    <div ref={ref} className={className}>
      {children}
      <Canvas eventSource={ref} eventPrefix="client" dpr={[1, 1.5]} style={STAGE_STYLE}>
        <View.Port />
      </Canvas>
    </div>
  );
}

// No z-index: dialogs and sticky bars with one keep stacking above the
// previews, while plain card backgrounds paint underneath them.
const STAGE_STYLE: CSSProperties = { position: "fixed", inset: 0, pointerEvents: "none" };

export interface CosmeticPreviewProps {
  slot: string;
  itemId: string;
  manifest: Manifest;
  /** Seat colour for tokens whose manifest does not fix one. */
  color?: string;
  className?: string;
  style?: CSSProperties;
}

/**
 * A spinning 3D preview of any cosmetic, rendered straight from its
 * manifest so the admin panel can show an item before it is saved and the
 * shop can show it before it is bought.
 */
export function CosmeticPreview({ slot, itemId, manifest, color = "#38bdf8", className, style }: CosmeticPreviewProps) {
  return (
    <View className={className} style={style} data-testid={`${slot}-preview-${itemId}`}>
      {slot === "token" && <TokenScene manifest={manifest} itemId={itemId} color={color} />}
      {slot === "board" && <BoardScene manifest={manifest} itemId={itemId} />}
      {slot === "dice" && <DiceScene manifest={manifest} itemId={itemId} />}
      {slot === "buildings" && <BuildingsScene manifest={manifest} itemId={itemId} color={color} />}
      {slot === "cards" && <CardsScene manifest={manifest} itemId={itemId} />}
    </View>
  );
}

type SceneProps = { manifest: Manifest; itemId: string; color?: string };

function TokenScene({ manifest, itemId, color }: SceneProps) {
  const skin = resolveToken({ [itemId]: manifest }, itemId, 0);
  return (
    <Turntable distance={1.84} tilt={0.51}>
      <group position={[0, -0.25, 0]}>
        <TokenMesh skin={skin} color={color!} castShadow={false} />
      </group>
    </Turntable>
  );
}

function BoardScene({ manifest, itemId }: SceneProps) {
  const skin = resolveBoard({ [itemId]: manifest }, itemId);
  return (
    <Turntable distance={19} tilt={0.9} speed={0.25} light={[6, 14, 8]}>
      <BoardMiniature skin={skin} />
    </Turntable>
  );
}

function DiceScene({ manifest, itemId }: SceneProps) {
  const skin = resolveDice({ [itemId]: manifest }, itemId);
  const materials = useMemo(() => dieMaterials(skin), [skin.body, skin.pip]);
  useEffect(
    () => () => {
      for (const m of materials) {
        m.map?.dispose();
        m.dispose();
      }
    },
    [materials],
  );
  return (
    <Turntable distance={2.1} tilt={0.75} speed={0.5}>
      <Die materials={materials} face={5} position={[-0.4, 0, 0.05]} twist={0.35} />
      <Die materials={materials} face={3} position={[0.4, 0, -0.05]} twist={-0.5} />
    </Turntable>
  );
}

function Die({ materials, face, position, twist }: { materials: THREE.Material[]; face: number; position: [number, number, number]; twist: number }) {
  return (
    <group position={position} rotation={[0, twist, 0]}>
      <mesh material={materials} rotation={UP_ROTATION[face]}>
        <boxGeometry args={[0.5, 0.5, 0.5]} />
      </mesh>
    </group>
  );
}

/**
 * Buildings are drawn by the board skin at the table today; a buildings
 * item is either a model (previewed like a token) or the builtin house and
 * hotel.
 */
function BuildingsScene({ manifest, itemId, color }: SceneProps) {
  if (manifest.model) {
    const skin = resolveToken({ [itemId]: manifest }, itemId, 0);
    return (
      <Turntable distance={1.84} tilt={0.51}>
        <group position={[0, -0.25, 0]}>
          <TokenMesh skin={skin} color={color!} castShadow={false} />
        </group>
      </Turntable>
    );
  }
  const { house, hotel } = BOARDS["board.classic"];
  return (
    <Turntable distance={2} tilt={0.55} speed={0.5}>
      <group scale={2}>
        <mesh position={[-0.32, 0.07, 0]}>
          <boxGeometry args={[0.16, 0.14, 0.16]} />
          <meshStandardMaterial color={house} />
        </mesh>
        <mesh position={[0.22, 0.1, 0]}>
          <boxGeometry args={[0.5, 0.2, 0.22]} />
          <meshStandardMaterial color={hotel} />
        </mesh>
      </group>
    </Turntable>
  );
}

function CardsScene({ manifest, itemId }: SceneProps) {
  const skin = resolveCards({ [itemId]: manifest }, itemId);
  return (
    <Turntable distance={2.3} tilt={0.85} speed={0.4}>
      <mesh position={[-0.24, 0, 0.03]} rotation={[0, 0.14, 0]}>
        <boxGeometry args={[0.62, 0.02, 0.9]} />
        <meshStandardMaterial color={skin.back} roughness={0.6} />
      </mesh>
      <mesh position={[0.24, 0.03, -0.03]} rotation={[0, -0.14, 0]}>
        <boxGeometry args={[0.62, 0.02, 0.9]} />
        <meshStandardMaterial color={skin.face} roughness={0.7} />
      </mesh>
    </Turntable>
  );
}

/**
 * The camera looks straight down -z; the content is tilted toward it and
 * spun about its own axis, so every slot frames the same way without
 * per-view lookAt bookkeeping. Each View is its own scene, so the lights
 * live here too.
 */
function Turntable({
  distance,
  tilt,
  fov = 35,
  speed = 0.8,
  light = [2, 3, 2],
  children,
}: {
  distance: number;
  tilt: number;
  fov?: number;
  speed?: number;
  light?: [number, number, number];
  children: ReactNode;
}) {
  return (
    <>
      <PerspectiveCamera makeDefault position={[0, 0, distance]} fov={fov} />
      <ambientLight intensity={0.6} />
      <directionalLight position={light} intensity={1.4} />
      <group rotation={[tilt, 0, 0]}>
        <Spinner speed={speed}>{children}</Spinner>
      </group>
    </>
  );
}

function Spinner({ speed, children }: { speed: number; children: ReactNode }) {
  const ref = useRef<THREE.Group>(null);
  useFrame((_, dt) => {
    if (ref.current) ref.current.rotation.y += dt * speed;
  });
  return <group ref={ref}>{children}</group>;
}

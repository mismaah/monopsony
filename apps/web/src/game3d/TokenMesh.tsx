import { Component, Suspense, useMemo, type ReactNode } from "react";
import * as THREE from "three";
import { useGLTF } from "@react-three/drei";
import type { ModelRef, TokenSkin } from "./skins";
import tophatUrl from "@monopsony/board-assets/models/tophat.glb?url";
import rocketUrl from "@monopsony/board-assets/models/rocket.glb?url";

/** Models bundled with the client; manifests reference them by name. */
export const BUILTIN_MODELS: Record<string, string> = { tophat: tophatUrl, rocket: rocketUrl };

export function modelURL(model: ModelRef | undefined): string | null {
  if (!model) return null;
  if (model.url) return model.url;
  return BUILTIN_MODELS[model.builtin ?? ""] ?? null;
}

interface Props {
  skin: TokenSkin;
  /** Seat colour: used unless the skin fixes its own colour. */
  color: string;
  /** Highlight (active player) glow. */
  active?: boolean;
  castShadow?: boolean;
}

/**
 * TokenMesh renders a token skin: a glTF model when the manifest has one
 * (falling back to the builtin shape while it loads or if it fails), or one
 * of the procedural builtin shapes.
 */
export function TokenMesh({ skin, color, active = false, castShadow = true }: Props) {
  const url = modelURL(skin.model);
  const shape = <BuiltinShape skin={skin} color={color} active={active} castShadow={castShadow} />;
  if (!url) return shape;
  return (
    <ModelBoundary fallback={shape}>
      <Suspense fallback={shape}>
        <GltfToken url={url} model={skin.model!} skin={skin} color={color} active={active} castShadow={castShadow} />
      </Suspense>
    </ModelBoundary>
  );
}

function GltfToken({ url, model, skin, color, active, castShadow }: Props & { url: string; model: ModelRef }) {
  const { scene } = useGLTF(url, model.draco ? true : undefined);
  const tint = skin.color ?? color;
  const object = useMemo(() => {
    const clone = scene.clone(true);
    clone.traverse((o) => {
      if (!(o as THREE.Mesh).isMesh) return;
      const mesh = o as THREE.Mesh;
      mesh.castShadow = castShadow ?? true;
      const src = Array.isArray(mesh.material) ? mesh.material[0] : mesh.material;
      const mat = (src as THREE.MeshStandardMaterial).clone();
      // Materials named keep_* hold their authored colour (trim, glass);
      // everything else is tinted with the seat colour so players can tell
      // tokens apart.
      if (!mat.name.startsWith("keep")) {
        mat.color = new THREE.Color(tint);
        if (skin.metalness !== undefined) mat.metalness = skin.metalness;
        if (skin.roughness !== undefined) mat.roughness = skin.roughness;
      }
      mat.emissive = new THREE.Color(active ? tint : skin.emissive ?? "#000");
      mat.emissiveIntensity = active ? 0.35 : 0.15;
      mesh.material = mat;
    });
    return clone;
  }, [scene, tint, active, skin.metalness, skin.roughness, skin.emissive, castShadow]);
  return <primitive object={object} scale={model.scale || 1} position={model.offset ?? [0, 0, 0]} rotation={model.rotation ?? [0, 0, 0]} />;
}

/** The procedural shapes: cheap, always available, tinted per seat. */
export function BuiltinShape({ skin, color, active = false, castShadow = true }: Props) {
  const mat = (
    <meshStandardMaterial
      color={skin.color ?? color}
      metalness={skin.metalness}
      roughness={skin.roughness}
      emissive={active ? color : skin.emissive ?? "#000"}
      emissiveIntensity={active ? 0.35 : 0.15}
    />
  );
  switch (skin.shape) {
    case "cone":
      return (
        <mesh castShadow={castShadow} position={[0, 0.25, 0]}>
          <coneGeometry args={[0.2, 0.5, 24]} />
          {mat}
        </mesh>
      );
    case "sphere":
      return (
        <mesh castShadow={castShadow} position={[0, 0.2, 0]}>
          <sphereGeometry args={[0.2, 32, 24]} />
          {mat}
        </mesh>
      );
    case "ring":
      return (
        <mesh castShadow={castShadow} position={[0, 0.22, 0]} rotation={[Math.PI / 2, 0, 0]}>
          <torusGeometry args={[0.16, 0.06, 16, 32]} />
          {mat}
        </mesh>
      );
    case "cube":
      return (
        <mesh castShadow={castShadow} position={[0, 0.17, 0]} rotation={[0, Math.PI / 4, 0]}>
          <boxGeometry args={[0.3, 0.3, 0.3]} />
          {mat}
        </mesh>
      );
    case "gem":
      return (
        <mesh castShadow={castShadow} position={[0, 0.25, 0]}>
          <octahedronGeometry args={[0.24, 0]} />
          {mat}
        </mesh>
      );
    default:
      return (
        <group>
          <mesh castShadow={castShadow} position={[0, 0.06, 0]}>
            <cylinderGeometry args={[0.16, 0.2, 0.12, 24]} />
            {mat}
          </mesh>
          <mesh castShadow={castShadow} position={[0, 0.26, 0]}>
            <cylinderGeometry args={[0.06, 0.12, 0.3, 24]} />
            {mat}
          </mesh>
          <mesh castShadow={castShadow} position={[0, 0.5, 0]}>
            <sphereGeometry args={[0.12, 24, 16]} />
            {mat}
          </mesh>
        </group>
      );
  }
}

/** Falls back to the builtin shape when a model fails to load (404, bad file). */
class ModelBoundary extends Component<{ fallback: ReactNode; children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  componentDidCatch(err: unknown) {
    console.warn("token model failed to load, using builtin shape", err);
  }
  render() {
    return this.state.failed ? this.props.fallback : this.props.children;
  }
}

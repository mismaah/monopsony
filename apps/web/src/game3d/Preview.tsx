import { Canvas, useFrame } from "@react-three/fiber";
import { useRef } from "react";
import * as THREE from "three";
import { resolveToken, type Manifest } from "./skins";
import { TokenMesh } from "./TokenMesh";
import { useCosmetics } from "@/store/cosmetics";

/**
 * Small spinning 3D preview of a token item (shop cards, lobby seats). The
 * manifest can be passed directly (admin/shop editing) or looked up by id.
 */
export function TokenPreview({ itemId, manifest, color }: { itemId: string; manifest?: Manifest; color?: string }) {
  const manifests = useCosmetics((s) => s.manifests);
  const skin = resolveToken(manifest ? { [itemId]: manifest } : manifests, itemId, 0);
  const c = color ?? "#38bdf8";
  return (
    <Canvas camera={{ position: [0, 0.9, 1.6], fov: 35 }} dpr={[1, 1.5]} data-testid={`token-preview-${itemId}`}>
      <ambientLight intensity={0.6} />
      <directionalLight position={[2, 3, 2]} intensity={1.4} />
      <Spinner>
        <group position={[0, -0.25, 0]}>
          <TokenMesh skin={skin} color={c} castShadow={false} />
        </group>
      </Spinner>
    </Canvas>
  );
}

function Spinner({ children }: { children: React.ReactNode }) {
  const ref = useRef<THREE.Group>(null);
  useFrame((_, dt) => {
    if (ref.current) ref.current.rotation.y += dt * 0.8;
  });
  return <group ref={ref}>{children}</group>;
}

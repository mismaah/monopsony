import * as THREE from "three";
import type { DiceSkin } from "./skins";

// Material order on a BoxGeometry is +x, -x, +y, -y, +z, -z. Opposite faces
// sum to 7, as on a real die.
export const FACE_BY_SIDE = [3, 4, 1, 6, 2, 5];

/** Euler rotation that brings the given face to +y (up). */
export const UP_ROTATION: Record<number, [number, number, number]> = {
  1: [0, 0, 0],
  6: [Math.PI, 0, 0],
  2: [-Math.PI / 2, 0, 0],
  5: [Math.PI / 2, 0, 0],
  3: [0, 0, Math.PI / 2],
  4: [0, 0, -Math.PI / 2],
};

const PIPS: Record<number, [number, number][]> = {
  1: [[0.5, 0.5]],
  2: [
    [0.25, 0.25],
    [0.75, 0.75],
  ],
  3: [
    [0.25, 0.25],
    [0.5, 0.5],
    [0.75, 0.75],
  ],
  4: [
    [0.25, 0.25],
    [0.75, 0.25],
    [0.25, 0.75],
    [0.75, 0.75],
  ],
  5: [
    [0.25, 0.25],
    [0.75, 0.25],
    [0.5, 0.5],
    [0.25, 0.75],
    [0.75, 0.75],
  ],
  6: [
    [0.25, 0.2],
    [0.75, 0.2],
    [0.25, 0.5],
    [0.75, 0.5],
    [0.25, 0.8],
    [0.75, 0.8],
  ],
};

function faceTexture(n: number, skin: DiceSkin): THREE.CanvasTexture {
  const c = document.createElement("canvas");
  c.width = c.height = 128;
  const ctx = c.getContext("2d")!;
  ctx.fillStyle = skin.body;
  ctx.fillRect(0, 0, 128, 128);
  ctx.fillStyle = skin.pip;
  for (const [x, y] of PIPS[n]) {
    ctx.beginPath();
    ctx.arc(x * 128, y * 128, 11, 0, Math.PI * 2);
    ctx.fill();
  }
  const tex = new THREE.CanvasTexture(c);
  tex.colorSpace = THREE.SRGBColorSpace;
  return tex;
}

/** One material per box side, pips painted in the skin's colours. */
export function dieMaterials(skin: DiceSkin): THREE.MeshStandardMaterial[] {
  return FACE_BY_SIDE.map((n) => new THREE.MeshStandardMaterial({ map: faceTexture(n, skin), roughness: 0.35, metalness: 0.05 }));
}

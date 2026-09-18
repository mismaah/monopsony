// Generates the bundled token models into models/*.glb. Run `npm run build`
// here after editing; the outputs are committed so the client works from
// a fresh checkout without a build step.
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { box, lathe, primitive, readGLB, writeGLB } from "./glb.mjs";

const out = join(dirname(fileURLToPath(import.meta.url)), "..", "models");
mkdirSync(out, { recursive: true });

function tophat() {
  // Materials: 0 = felt (tinted with the seat colour), 1 = band (kept).
  const materials = [
    { name: "felt", color: [0.16, 0.16, 0.18], roughness: 0.7 },
    { name: "keep_band", color: [0.85, 0.66, 0.2], metallic: 0.6, roughness: 0.35 },
  ];
  const brim = lathe(primitive(0), [
    [0, 0],
    [0.21, 0],
    [0.21, 0.025],
    [0.13, 0.025],
  ]);
  const crown = lathe(primitive(0), [
    [0.13, 0.025],
    [0.135, 0.32],
    [0, 0.32],
  ]);
  const band = lathe(primitive(1), [
    [0.138, 0.04],
    [0.138, 0.09],
    [0.13, 0.09],
    [0.13, 0.04],
  ]);
  return writeGLB({ name: "tophat", primitives: [brim, crown, band], materials });
}

function rocket() {
  // 0 = hull (tinted), 1 = nose/fins (kept), 2 = porthole (kept).
  const materials = [
    { name: "hull", color: [0.85, 0.85, 0.88], metallic: 0.5, roughness: 0.35 },
    { name: "keep_trim", color: [0.8, 0.15, 0.15], metallic: 0.2, roughness: 0.5 },
    { name: "keep_glass", color: [0.3, 0.7, 0.95], metallic: 0.1, roughness: 0.2 },
  ];
  const hull = lathe(primitive(0), [
    [0, 0.06],
    [0.09, 0.06],
    [0.11, 0.12],
    [0.11, 0.36],
  ]);
  const nose = lathe(primitive(1), [
    [0.11, 0.36],
    [0.04, 0.5],
    [0, 0.54],
  ]);
  const nozzle = lathe(primitive(1), [
    [0, 0],
    [0.07, 0],
    [0.09, 0.06],
    [0, 0.06],
  ]);
  const port = lathe(primitive(2), [
    [0.03, 0.115],
    [0.03, 0.23],
    [0, 0.23],
  ]);
  // Rotate the porthole out to the hull surface by building it as a box
  // instead; a small flat disc on the +X side is enough at token scale.
  port.positions = [];
  port.normals = [];
  port.indices = [];
  box(port, 0.02, 0.06, 0.06, { translate: [0.105, 0.2, 0] });
  const fins = primitive(1);
  for (let i = 0; i < 3; i++) {
    box(fins, 0.02, 0.14, 0.1, { rotateY: (i * Math.PI * 2) / 3, translate: [0.1 * Math.cos((i * Math.PI * 2) / 3), 0.02, -0.1 * Math.sin((i * Math.PI * 2) / 3)] });
  }
  return writeGLB({ name: "rocket", primitives: [hull, nose, nozzle, port, fins], materials });
}

for (const [name, make] of Object.entries({ tophat, rocket })) {
  const glb = make();
  const { json, binLength } = readGLB(glb); // self-check
  const tris = json.meshes[0].primitives.reduce((n, p) => n + json.accessors[p.indices].count / 3, 0);
  writeFileSync(join(out, `${name}.glb`), glb);
  console.log(`${name}.glb  ${glb.length} bytes, ${tris} triangles, bin ${binLength} bytes`);
}

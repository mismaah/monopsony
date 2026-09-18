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

function crown() {
  // 0 = band and points (tinted), 1 = jewels (kept).
  const materials = [
    { name: "band", color: [0.9, 0.75, 0.3], metallic: 0.8, roughness: 0.3 },
    { name: "keep_jewel", color: [0.85, 0.12, 0.25], metallic: 0.2, roughness: 0.15 },
  ];
  // Hollow ring with a floor so the board does not show through.
  const band = lathe(primitive(0), [
    [0.13, 0],
    [0.18, 0],
    [0.18, 0.14],
    [0.13, 0.14],
    [0.13, 0],
  ]);
  const floor = lathe(primitive(0), [
    [0, 0.04],
    [0.13, 0.04],
  ]);
  const points = primitive(0);
  const jewels = primitive(1);
  const n = 6;
  for (let i = 0; i < n; i++) {
    const a = (i * Math.PI * 2) / n;
    box(points, 0.05, 0.14, 0.06, { rotateY: a, translate: [0.155 * Math.cos(a), 0.14, -0.155 * Math.sin(a)] });
    const j = a + Math.PI / n;
    box(jewels, 0.02, 0.04, 0.04, { rotateY: j, translate: [0.18 * Math.cos(j), 0.05, -0.18 * Math.sin(j)] });
  }
  return writeGLB({ name: "crown", primitives: [band, floor, points, jewels], materials });
}

function trophy() {
  // 0 = cup and handles (tinted), 1 = base (kept), 2 = name plate (kept).
  const materials = [
    { name: "cup", color: [0.9, 0.78, 0.35], metallic: 0.9, roughness: 0.25 },
    { name: "keep_base", color: [0.12, 0.1, 0.1], metallic: 0.1, roughness: 0.5 },
    { name: "keep_plate", color: [0.85, 0.66, 0.2], metallic: 0.6, roughness: 0.35 },
  ];
  const base = lathe(primitive(1), [
    [0, 0],
    [0.16, 0],
    [0.16, 0.05],
    [0.12, 0.05],
    [0.12, 0.08],
    [0, 0.08],
  ]);
  // Stem, then the outside of the bowl, over the lip and down the inside.
  const cup = lathe(primitive(0), [
    [0, 0.08],
    [0.06, 0.08],
    [0.03, 0.12],
    [0.03, 0.2],
    [0.09, 0.24],
    [0.12, 0.32],
    [0.14, 0.42],
    [0.12, 0.42],
    [0.1, 0.32],
    [0.07, 0.26],
    [0, 0.25],
  ]);
  const handles = primitive(0);
  for (const side of [1, -1]) {
    box(handles, 0.03, 0.14, 0.03, { translate: [side * 0.17, 0.27, 0] });
    box(handles, 0.07, 0.03, 0.03, { translate: [side * 0.14, 0.38, 0] });
    box(handles, 0.07, 0.03, 0.03, { translate: [side * 0.14, 0.27, 0] });
  }
  const plate = box(primitive(2), 0.07, 0.03, 0.01, { translate: [0, 0.01, 0.16] });
  return writeGLB({ name: "trophy", primitives: [base, cup, handles, plate], materials });
}

function car() {
  // 0 = bodywork (tinted), 1 = glass (kept), 2 = tyres (kept), 3 = lamps (kept).
  const materials = [
    { name: "body", color: [0.8, 0.2, 0.2], metallic: 0.5, roughness: 0.4 },
    { name: "keep_glass", color: [0.3, 0.7, 0.95], metallic: 0.1, roughness: 0.2 },
    { name: "keep_tyre", color: [0.08, 0.08, 0.09], roughness: 0.9 },
    { name: "keep_lamp", color: [1, 0.95, 0.7], roughness: 0.3 },
  ];
  const body = primitive(0);
  box(body, 0.4, 0.1, 0.2, { translate: [0, 0.05, 0] });
  box(body, 0.2, 0.09, 0.18, { translate: [-0.02, 0.15, 0] });
  const glass = primitive(1);
  box(glass, 0.01, 0.07, 0.16, { translate: [0.085, 0.16, 0] }); // windscreen
  box(glass, 0.01, 0.07, 0.16, { translate: [-0.125, 0.16, 0] }); // rear window
  for (const side of [1, -1]) box(glass, 0.14, 0.06, 0.01, { translate: [-0.02, 0.165, side * 0.095] });
  const tyres = primitive(2);
  for (const x of [0.13, -0.13]) for (const z of [0.105, -0.105]) box(tyres, 0.08, 0.08, 0.03, { translate: [x, 0, z] });
  const lamps = primitive(3);
  for (const z of [0.06, -0.06]) box(lamps, 0.01, 0.03, 0.04, { translate: [0.2, 0.06, z] });
  return writeGLB({ name: "car", primitives: [body, glass, tyres, lamps], materials });
}

function king() {
  // 0 = piece (tinted), 1 = cross (kept).
  const materials = [
    { name: "piece", color: [0.2, 0.2, 0.22], metallic: 0.3, roughness: 0.5 },
    { name: "keep_cross", color: [0.85, 0.66, 0.2], metallic: 0.6, roughness: 0.35 },
  ];
  const piece = lathe(primitive(0), [
    [0, 0],
    [0.17, 0],
    [0.17, 0.03],
    [0.13, 0.05],
    [0.08, 0.1],
    [0.06, 0.2],
    [0.06, 0.3],
    [0.1, 0.36],
    [0.12, 0.42],
    [0.06, 0.46],
    [0.04, 0.48],
    [0, 0.48],
  ]);
  const cross = primitive(1);
  box(cross, 0.025, 0.1, 0.025, { translate: [0, 0.47, 0] });
  box(cross, 0.08, 0.025, 0.025, { translate: [0, 0.51, 0] });
  return writeGLB({ name: "king", primitives: [piece, cross], materials });
}

for (const [name, make] of Object.entries({ tophat, rocket, crown, trophy, car, king })) {
  const glb = make();
  const { json, binLength } = readGLB(glb); // self-check
  const tris = json.meshes[0].primitives.reduce((n, p) => n + json.accessors[p.indices].count / 3, 0);
  writeFileSync(join(out, `${name}.glb`), glb);
  console.log(`${name}.glb  ${glb.length} bytes, ${tris} triangles, bin ${binLength} bytes`);
}

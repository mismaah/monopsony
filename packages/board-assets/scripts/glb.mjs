// Minimal glTF 2.0 binary (.glb) writer with a few procedural primitives.
// No dependencies: the goal is that `npm run build` in this package always
// works and that the shape of a valid token model is documented in code.
//
// Coordinate system: Y up, 1 unit = one board tile (~0.4 units is the
// token footprint the client expects). Origin at the bottom centre.

const GLB_MAGIC = 0x46546c67; // "glTF"
const CHUNK_JSON = 0x4e4f534a;
const CHUNK_BIN = 0x004e4942;

/** A primitive: flat arrays plus the material that colours it. */
export function primitive(material) {
  return { positions: [], normals: [], indices: [], material };
}

/**
 * Revolve a 2-D profile (array of [r, y]) around the Y axis. Each profile
 * segment gets its own vertex rings so hard corners stay crisp.
 */
export function lathe(prim, profile, segments = 48) {
  for (let s = 0; s < profile.length - 1; s++) {
    const [r0, y0] = profile[s];
    const [r1, y1] = profile[s + 1];
    if (r0 === 0 && r1 === 0) continue;
    const dr = r1 - r0;
    const dy = y1 - y0;
    const len = Math.hypot(dr, dy) || 1;
    const nr = dy / len; // 2-D outward normal of the segment
    const ny = -dr / len;
    const base = prim.positions.length / 3;
    for (const [r, y] of [
      [r0, y0],
      [r1, y1],
    ]) {
      for (let j = 0; j <= segments; j++) {
        const t = (j / segments) * Math.PI * 2;
        const c = Math.cos(t);
        const sn = Math.sin(t);
        prim.positions.push(r * c, y, r * sn);
        prim.normals.push(nr * c, ny, nr * sn);
      }
    }
    const ring = segments + 1;
    for (let j = 0; j < segments; j++) {
      const a = base + j;
      const b = base + j + 1;
      const c = base + ring + j + 1;
      const d = base + ring + j;
      // Counter-clockwise from outside: a -> d -> c, a -> c -> b.
      prim.indices.push(a, d, c, a, c, b);
    }
  }
  return prim;
}

/** Axis-aligned box centred at (0, h/2, 0), then transformed. */
export function box(prim, w, h, d, { rotateY = 0, translate = [0, 0, 0] } = {}) {
  const x = w / 2;
  const z = d / 2;
  const faces = [
    { n: [1, 0, 0], v: [[x, 0, z], [x, 0, -z], [x, h, -z], [x, h, z]] },
    { n: [-1, 0, 0], v: [[-x, 0, -z], [-x, 0, z], [-x, h, z], [-x, h, -z]] },
    { n: [0, 1, 0], v: [[-x, h, z], [x, h, z], [x, h, -z], [-x, h, -z]] },
    { n: [0, -1, 0], v: [[-x, 0, -z], [x, 0, -z], [x, 0, z], [-x, 0, z]] },
    { n: [0, 0, 1], v: [[-x, 0, z], [x, 0, z], [x, h, z], [-x, h, z]] },
    { n: [0, 0, -1], v: [[x, 0, -z], [-x, 0, -z], [-x, h, -z], [x, h, -z]] },
  ];
  const cs = Math.cos(rotateY);
  const sn = Math.sin(rotateY);
  const rot = ([px, py, pz]) => [px * cs + pz * sn, py, -px * sn + pz * cs];
  for (const f of faces) {
    const base = prim.positions.length / 3;
    const n = rot(f.n);
    for (const v of f.v) {
      const p = rot(v);
      prim.positions.push(p[0] + translate[0], p[1] + translate[1], p[2] + translate[2]);
      prim.normals.push(n[0], n[1], n[2]);
    }
    prim.indices.push(base, base + 1, base + 2, base, base + 2, base + 3);
  }
  return prim;
}

/**
 * Pack primitives into a GLB. Materials: { name, color: [r,g,b], metallic,
 * roughness }. A material whose name starts with "keep" is not tinted with
 * the seat colour by the client.
 */
export function writeGLB({ name, primitives, materials }) {
  const bin = [];
  let byteOffset = 0;
  const bufferViews = [];
  const accessors = [];
  const meshPrims = [];

  const pushView = (bytes, target) => {
    // Every view starts 4-byte aligned.
    const pad = (4 - (byteOffset % 4)) % 4;
    if (pad) {
      bin.push(Buffer.alloc(pad));
      byteOffset += pad;
    }
    bufferViews.push({ buffer: 0, byteOffset, byteLength: bytes.length, target });
    bin.push(bytes);
    byteOffset += bytes.length;
    return bufferViews.length - 1;
  };

  for (const prim of primitives) {
    const count = prim.positions.length / 3;
    const pos = Float32Array.from(prim.positions);
    const nor = Float32Array.from(prim.normals);
    const useInt = count > 65535;
    const idx = useInt ? Uint32Array.from(prim.indices) : Uint16Array.from(prim.indices);
    const min = [Infinity, Infinity, Infinity];
    const max = [-Infinity, -Infinity, -Infinity];
    for (let i = 0; i < count; i++) {
      for (let k = 0; k < 3; k++) {
        min[k] = Math.min(min[k], pos[i * 3 + k]);
        max[k] = Math.max(max[k], pos[i * 3 + k]);
      }
    }
    const posView = pushView(Buffer.from(pos.buffer), 34962);
    const norView = pushView(Buffer.from(nor.buffer), 34962);
    const idxView = pushView(Buffer.from(idx.buffer), 34963);
    accessors.push({ bufferView: posView, componentType: 5126, count, type: "VEC3", min, max });
    accessors.push({ bufferView: norView, componentType: 5126, count, type: "VEC3" });
    accessors.push({ bufferView: idxView, componentType: useInt ? 5125 : 5123, count: idx.length, type: "SCALAR" });
    const a = accessors.length - 3;
    meshPrims.push({ attributes: { POSITION: a, NORMAL: a + 1 }, indices: a + 2, material: prim.material });
  }

  const json = {
    asset: { version: "2.0", generator: "monopsony board-assets" },
    scene: 0,
    scenes: [{ nodes: [0] }],
    nodes: [{ mesh: 0, name }],
    meshes: [{ name, primitives: meshPrims }],
    materials: materials.map((m) => ({
      name: m.name,
      pbrMetallicRoughness: { baseColorFactor: [...m.color, 1], metallicFactor: m.metallic ?? 0, roughnessFactor: m.roughness ?? 0.6 },
    })),
    buffers: [{ byteLength: byteOffset }],
    bufferViews,
    accessors,
  };

  let jsonBytes = Buffer.from(JSON.stringify(json), "utf8");
  const jsonPad = (4 - (jsonBytes.length % 4)) % 4;
  if (jsonPad) jsonBytes = Buffer.concat([jsonBytes, Buffer.alloc(jsonPad, 0x20)]);
  let binBytes = Buffer.concat(bin);
  const binPad = (4 - (binBytes.length % 4)) % 4;
  if (binPad) binBytes = Buffer.concat([binBytes, Buffer.alloc(binPad)]);

  const header = Buffer.alloc(12);
  header.writeUInt32LE(GLB_MAGIC, 0);
  header.writeUInt32LE(2, 4);
  header.writeUInt32LE(12 + 8 + jsonBytes.length + 8 + binBytes.length, 8);
  const chunk = (type, bytes) => {
    const h = Buffer.alloc(8);
    h.writeUInt32LE(bytes.length, 0);
    h.writeUInt32LE(type, 4);
    return Buffer.concat([h, bytes]);
  };
  return Buffer.concat([header, chunk(CHUNK_JSON, jsonBytes), chunk(CHUNK_BIN, binBytes)]);
}

/** Parse a GLB header + JSON chunk (validation / inspection). */
export function readGLB(buf) {
  if (buf.length < 20 || buf.readUInt32LE(0) !== GLB_MAGIC) throw new Error("not a GLB (bad magic)");
  const version = buf.readUInt32LE(4);
  const total = buf.readUInt32LE(8);
  if (version !== 2) throw new Error(`unsupported glTF version ${version}`);
  if (total !== buf.length) throw new Error(`length mismatch: header says ${total}, file is ${buf.length}`);
  const jsonLen = buf.readUInt32LE(12);
  if (buf.readUInt32LE(16) !== CHUNK_JSON) throw new Error("first chunk is not JSON");
  const json = JSON.parse(buf.subarray(20, 20 + jsonLen).toString("utf8"));
  let binLength = 0;
  const binStart = 20 + jsonLen;
  if (binStart + 8 <= buf.length && buf.readUInt32LE(binStart + 4) === CHUNK_BIN) binLength = buf.readUInt32LE(binStart);
  return { json, binLength };
}

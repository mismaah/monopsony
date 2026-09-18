#!/usr/bin/env node
// Inspect a .glb before uploading it: prints size, meshes, triangle count,
// extensions and warns about things the client cannot handle.
//
//   node scripts/check-glb.mjs path/to/model.glb
import { readFileSync } from "node:fs";
import { readGLB } from "./glb.mjs";

const file = process.argv[2];
if (!file) {
  console.error("usage: check-glb <file.glb>");
  process.exit(2);
}
const buf = readFileSync(file);
const { json, binLength } = readGLB(buf);
const warnings = [];
const tris = (json.meshes ?? []).reduce(
  (n, m) => n + m.primitives.reduce((k, p) => k + (p.indices !== undefined ? json.accessors[p.indices].count / 3 : json.accessors[p.attributes.POSITION].count / 3), 0),
  0,
);
const ext = json.extensionsRequired ?? [];
const known = new Set(["KHR_draco_mesh_compression", "KHR_materials_emissive_strength", "KHR_texture_transform", "KHR_mesh_quantization"]);
for (const e of ext) if (!known.has(e)) warnings.push(`requires extension ${e}, which the client may not support`);
if (buf.length > 2 * 1024 * 1024) warnings.push("larger than 2 MB: run `npm run optimize` (Draco) before uploading");
if (tris > 50_000) warnings.push(`${tris} triangles is heavy for a token; aim for < 10k`);
if ((json.images ?? []).some((i) => i.uri && !i.uri.startsWith("data:"))) warnings.push("references external image files; embed textures (GLB) so a single upload is enough");
for (const m of json.meshes ?? []) for (const p of m.primitives) if (!p.attributes.NORMAL) warnings.push(`mesh ${m.name ?? "?"} has no normals (lighting will look flat)`);

let minY = Infinity;
let maxY = -Infinity;
let radius = 0;
for (const m of json.meshes ?? [])
  for (const p of m.primitives) {
    const a = json.accessors[p.attributes.POSITION];
    if (a.min && a.max) {
      minY = Math.min(minY, a.min[1]);
      maxY = Math.max(maxY, a.max[1]);
      radius = Math.max(radius, Math.abs(a.min[0]), Math.abs(a.max[0]), Math.abs(a.min[2]), Math.abs(a.max[2]));
    }
  }
console.log(`${file}: ${buf.length} bytes (bin ${binLength}), ${json.meshes?.length ?? 0} mesh(es), ${tris} triangles`);
console.log(`materials: ${(json.materials ?? []).map((m) => m.name ?? "?").join(", ") || "none"}`);
if (ext.length) console.log(`required extensions: ${ext.join(", ")}`);
if (Number.isFinite(minY)) {
  console.log(`bounds: y ${minY.toFixed(3)}..${maxY.toFixed(3)}, radius ${radius.toFixed(3)} (token footprint is ~0.2 radius, 0.5 tall; set manifest model.scale/offset to fit)`);
  if (minY < -0.01) warnings.push("model extends below y=0; set model.offset or re-export with the base at the origin");
}
for (const w of warnings) console.log(`warning: ${w}`);
process.exit(warnings.length ? 1 : 0);

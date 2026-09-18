#!/usr/bin/env node
// Upload one or more asset files to a running server's admin API and print
// the manifest snippet to paste into the cosmetics editor.
//
//   MONOPSONY_URL=http://localhost:8080 MONOPSONY_ADMIN_EMAIL=... MONOPSONY_ADMIN_PASSWORD=... \
//     node scripts/upload.mjs models/tophat.glb textures/*.png
//
// or pass an access token directly with MONOPSONY_ADMIN_TOKEN.
import { readFileSync, statSync } from "node:fs";
import { basename } from "node:path";

const base = (process.env.MONOPSONY_URL ?? "http://localhost:8080").replace(/\/$/, "");
const files = process.argv.slice(2);
if (files.length === 0) {
  console.error("usage: upload <file>...");
  process.exit(2);
}

async function token() {
  if (process.env.MONOPSONY_ADMIN_TOKEN) return process.env.MONOPSONY_ADMIN_TOKEN;
  const email = process.env.MONOPSONY_ADMIN_EMAIL;
  const password = process.env.MONOPSONY_ADMIN_PASSWORD;
  if (!email || !password) throw new Error("set MONOPSONY_ADMIN_TOKEN or MONOPSONY_ADMIN_EMAIL + MONOPSONY_ADMIN_PASSWORD");
  const res = await fetch(`${base}/api/auth/login`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email, password }) });
  if (!res.ok) throw new Error(`login failed: ${res.status} ${await res.text()}`);
  return (await res.json()).accessToken;
}

const t = await token();
for (const f of files) {
  statSync(f);
  const form = new FormData();
  form.append("file", new Blob([readFileSync(f)]), basename(f));
  const res = await fetch(`${base}/admin/api/assets`, { method: "POST", headers: { Authorization: `Bearer ${t}` }, body: form });
  const body = await res.json();
  if (!res.ok) {
    console.error(`${f}: ${res.status} ${body?.error?.message ?? ""}`);
    process.exitCode = 1;
    continue;
  }
  const a = body.asset;
  console.log(`${f} -> ${a.url} (${a.size} bytes)`);
  if (a.url.endsWith(".glb") || a.url.endsWith(".gltf")) console.log(`  manifest: ${JSON.stringify({ model: { url: a.url, scale: 1 } })}`);
}

import { useEffect, useRef, useState } from "react";
import { api, getAccessToken, ApiError } from "@/lib/http";
import { Btn, Field, Notice, Panel } from "@/lib/ui";
import { CosmeticPreview, PreviewStage, type Manifest } from "@monopsony/cosmetics";

interface Cosmetic {
  id: string;
  slot: string;
  name: string;
  description: string;
  priceCents: number;
  currency: string;
  tierRequired?: string;
  manifest: Record<string, unknown>;
  enabled: boolean;
  sortOrder: number;
}

interface Asset {
  id: string;
  name: string;
  size: number;
  contentType: string;
  url: string;
  uploadedAt: string;
}

const SLOTS = ["token", "board", "dice", "buildings", "cards"];

/** Starting points for the manifest editor, one per slot. */
const templates: Record<string, Record<string, unknown>[]> = {
  token: [
    { builtin: "token.gem", color: "#7dd3fc" },
    { model: { builtin: "tophat" }, material: { metalness: 0.1, roughness: 0.6 } },
    { model: { builtin: "crown" }, material: { metalness: 0.45, roughness: 0.35 } },
    { model: { url: "/media/<uploaded>.glb", scale: 1, offset: [0, 0, 0], rotation: [0, 0, 0] } },
  ],
  board: [{ builtin: "board.midnight" }, { palette: { table: "#0e3a4a", tile: "#f1e9d2", tileEdge: "#082430", text: "#0f172a", centre: "#cfe6df", house: "#10b981", hotel: "#f5c451", mortgageTint: "#9a9385" } }],
  dice: [{ builtin: "dice.onyx" }, { palette: { body: "#111827", pip: "#f5f1e6" } }],
  buildings: [{ builtin: "buildings.classic" }],
  cards: [{ builtin: "cards.classic" }, { palette: { back: "#1e3a8a", face: "#f8fafc" } }],
};

const empty: Cosmetic = { id: "", slot: "token", name: "", description: "", priceCents: 0, currency: "usd", tierRequired: "", manifest: templates.token[0], enabled: true, sortOrder: 100 };

export default function CosmeticsPage() {
  const [items, setItems] = useState<Cosmetic[]>([]);
  const [edit, setEdit] = useState<Cosmetic | null>(null);
  const [manifest, setManifest] = useState("");
  // What the preview shows: the last manifest that parsed, a beat after typing
  // stops so half-typed model URLs are not fetched.
  const [live, setLive] = useState<Manifest | null>(null);
  const [check, setCheck] = useState<{ ok: boolean; error?: string } | null>(null);
  const [msg, setMsg] = useState<{ kind: "ok" | "err"; text: string } | null>(null);
  const load = () => api<{ items: Cosmetic[] }>("GET", "/admin/api/cosmetics").then((r) => setItems(r.items));
  useEffect(() => {
    void load();
  }, []);
  useEffect(() => {
    const t = setTimeout(() => {
      try {
        setLive(JSON.parse(manifest));
      } catch {
        /* keep the last good look */
      }
    }, 300);
    return () => clearTimeout(t);
  }, [manifest]);

  function open(c: Cosmetic) {
    setEdit(c);
    setManifest(JSON.stringify(c.manifest ?? {}, null, 2));
    setLive((c.manifest ?? {}) as Manifest);
    setCheck(null);
  }
  function parsed(): Record<string, unknown> | null {
    try {
      return JSON.parse(manifest);
    } catch {
      setCheck({ ok: false, error: "manifest is not valid JSON" });
      return null;
    }
  }
  async function validate() {
    if (!edit) return;
    const m = parsed();
    if (!m) return;
    const res = await api<{ ok: boolean; error?: string }>("POST", "/admin/api/cosmetics/validate", { slot: edit.slot, manifest: m });
    setCheck(res);
  }
  async function save() {
    if (!edit) return;
    const m = parsed();
    if (!m) return;
    try {
      await api("PUT", `/admin/api/cosmetics/${edit.id}`, { ...edit, manifest: m });
      setMsg({ kind: "ok", text: `${edit.name} saved.` });
      setEdit(null);
      await load();
    } catch (e) {
      setMsg({ kind: "err", text: (e as Error).message });
    }
  }

  return (
    <PreviewStage>
      <Panel title="Cosmetics" actions={<Btn onClick={() => open({ ...empty })}>+ New item</Btn>}>
        <p className="text-sm text-zinc-400 mb-3">
          The manifest tells the client how to render an item: a <code>builtin</code> look, a <code>palette</code> (board/dice/cards), or a glTF <code>model</code> — bundled (<code>{"{\"model\":{\"builtin\":\"tophat\"}}"}</code>) or uploaded below. Materials named <code>keep_*</code> in a model
          keep their colour; everything else is tinted with the seat colour.
        </p>
        {msg && (
          <div className="mb-3">
            <Notice kind={msg.kind} text={msg.text} />
          </div>
        )}
        {edit && (
          <div className="border border-emerald-500/40 rounded-lg p-4 mb-4 grid md:grid-cols-3 gap-3">
            <Field label="Id (immutable)">
              <input value={edit.id} disabled={items.some((i) => i.id === edit.id)} onChange={(e) => setEdit({ ...edit, id: e.target.value })} />
            </Field>
            <Field label="Slot">
              <select
                value={edit.slot}
                onChange={(e) => {
                  setEdit({ ...edit, slot: e.target.value });
                  setCheck(null);
                }}
              >
                {SLOTS.map((s) => (
                  <option key={s}>{s}</option>
                ))}
              </select>
            </Field>
            <Field label="Name">
              <input value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} />
            </Field>
            <Field label="Description">
              <input value={edit.description} onChange={(e) => setEdit({ ...edit, description: e.target.value })} />
            </Field>
            <Field label="Price (cents, 0 = free)">
              <input type="number" min={0} value={edit.priceCents} onChange={(e) => setEdit({ ...edit, priceCents: +e.target.value })} />
            </Field>
            <Field label="Tier required">
              <select value={edit.tierRequired ?? ""} onChange={(e) => setEdit({ ...edit, tierRequired: e.target.value })}>
                <option value="">any</option>
                <option value="premium">premium</option>
              </select>
            </Field>
            <Field label="Sort order">
              <input type="number" value={edit.sortOrder} onChange={(e) => setEdit({ ...edit, sortOrder: +e.target.value })} />
            </Field>
            <label className="flex items-center gap-2 text-sm mt-5">
              <input type="checkbox" checked={edit.enabled} onChange={(e) => setEdit({ ...edit, enabled: e.target.checked })} /> Enabled (visible in shop)
            </label>
            <div className="md:col-span-2">
              <div className="flex items-center gap-2 mb-1">
                <span className="text-zinc-400 text-xs uppercase tracking-wide">Manifest (JSON)</span>
                <span className="flex-1" />
                <span className="text-xs text-zinc-500">templates:</span>
                {(templates[edit.slot] ?? []).map((t, i) => (
                  <button key={i} className="text-xs px-2 py-0.5 rounded bg-zinc-800 hover:bg-zinc-700" onClick={() => setManifest(JSON.stringify(t, null, 2))}>
                    {t.model ? ((t.model as { builtin?: string }).builtin ? "bundled model" : "uploaded model") : t.palette ? "palette" : "builtin"}
                  </button>
                ))}
              </div>
              <textarea
                className="font-mono text-xs h-40 w-full"
                value={manifest}
                onChange={(e) => {
                  setManifest(e.target.value);
                  setCheck(null);
                }}
              />
              {check && <div className="mt-1">{check.ok ? <Notice kind="ok" text="Manifest is valid." /> : <Notice kind="err" text={check.error ?? "invalid"} />}</div>}
            </div>
            <div>
              <div className="text-zinc-400 text-xs uppercase tracking-wide mb-1">Preview</div>
              {live && <CosmeticPreview slot={edit.slot} itemId={edit.id || "new"} manifest={live} className="h-40 rounded bg-zinc-950" />}
              <p className="text-xs text-zinc-500 mt-1">Follows the manifest as you type; invalid JSON keeps the last good look.</p>
            </div>
            <div className="md:col-span-3 flex gap-2">
              <Btn tone="secondary" onClick={validate}>
                Validate
              </Btn>
              <Btn onClick={save}>Save</Btn>
              <Btn tone="secondary" onClick={() => setEdit(null)}>
                Cancel
              </Btn>
            </div>
          </div>
        )}
        <table className="w-full text-sm">
          <thead className="text-zinc-400">
            <tr>
              <th></th>
              <th>Id</th>
              <th>Slot</th>
              <th>Name</th>
              <th>Look</th>
              <th>Price</th>
              <th>Tier</th>
              <th>Enabled</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {items.map((c) => (
              <tr key={c.id} className="border-t border-zinc-800">
                <td className="py-1 pr-2">
                  <CosmeticPreview slot={c.slot} itemId={c.id} manifest={(c.manifest ?? {}) as Manifest} className="w-24 h-14 rounded bg-zinc-950" />
                </td>
                <td className="font-mono text-xs">{c.id}</td>
                <td>{c.slot}</td>
                <td>{c.name}</td>
                <td className="text-xs text-zinc-400">{describe(c.manifest)}</td>
                <td>{c.priceCents ? `$${(c.priceCents / 100).toFixed(2)}` : "free"}</td>
                <td>{c.tierRequired || "any"}</td>
                <td>{c.enabled ? "yes" : <span className="text-zinc-500">no</span>}</td>
                <td className="text-right">
                  <Btn tone="secondary" onClick={() => open(c)}>
                    Edit
                  </Btn>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </Panel>
      <AssetsPanel onPick={(url) => edit && setManifest(JSON.stringify({ model: { url, scale: 1 } }, null, 2))} />
    </PreviewStage>
  );
}

function describe(m: Record<string, unknown>): string {
  const model = m.model as { url?: string; builtin?: string } | undefined;
  if (model?.url) return `glTF ${model.url.split("/").pop()}`;
  if (model?.builtin) return `glTF (bundled ${model.builtin})`;
  if (m.palette) return "palette";
  return typeof m.builtin === "string" ? m.builtin : "—";
}

/** Uploaded models/textures, served under /media/ with immutable caching. */
function AssetsPanel({ onPick }: { onPick: (url: string) => void }) {
  const [assets, setAssets] = useState<Asset[] | null>(null);
  const [disabled, setDisabled] = useState(false);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ kind: "ok" | "err"; text: string } | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  async function load() {
    try {
      const r = await api<{ assets: Asset[] }>("GET", "/admin/api/assets");
      setAssets(r.assets);
    } catch (e) {
      if (e instanceof ApiError && e.status === 501) setDisabled(true);
      else setMsg({ kind: "err", text: (e as Error).message });
    }
  }
  useEffect(() => {
    void load();
  }, []);

  async function upload(files: FileList | null) {
    if (!files || files.length === 0) return;
    setBusy(true);
    setMsg(null);
    try {
      for (const f of Array.from(files)) {
        const form = new FormData();
        form.append("file", f, f.name);
        const res = await fetch("/admin/api/assets", { method: "POST", headers: { Authorization: `Bearer ${getAccessToken()}` }, body: form });
        const body = await res.json();
        if (!res.ok) throw new Error(body?.error?.message ?? `upload failed (${res.status})`);
      }
      setMsg({ kind: "ok", text: `${files.length} file(s) uploaded.` });
      await load();
    } catch (e) {
      setMsg({ kind: "err", text: (e as Error).message });
    } finally {
      setBusy(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  async function remove(a: Asset) {
    if (!confirm(`Delete ${a.name}? Items referencing it will fall back to their builtin shape.`)) return;
    await api("DELETE", `/admin/api/assets/${a.id}`);
    await load();
  }

  return (
    <Panel
      title="Assets"
      actions={
        <label className={`px-3 py-1 rounded text-sm font-medium cursor-pointer ${disabled || busy ? "opacity-40 pointer-events-none" : "bg-emerald-500 text-zinc-950 hover:bg-emerald-400"}`}>
          {busy ? "Uploading…" : "+ Upload"}
          <input ref={fileRef} type="file" multiple accept=".glb,.gltf,.bin,.png,.jpg,.jpeg,.webp,.ktx2,.hdr" className="hidden" onChange={(e) => upload(e.target.files)} />
        </label>
      }
    >
      <p className="text-sm text-zinc-400 mb-3">
        glTF binaries (.glb, textures embedded) up to 25 MB. Run <code>npm run check -w @monopsony/board-assets -- &lt;file&gt;</code> before uploading to catch missing normals or oversized meshes, and <code>npm run optimize -w @monopsony/board-assets -- in.glb out.glb</code> for Draco compression
        (set <code>"draco": true</code> in the manifest).
      </p>
      {disabled && <Notice kind="err" text="Uploads are disabled on this server (MONOPSONY_ASSET_DIR is off)." />}
      {msg && (
        <div className="mb-3">
          <Notice kind={msg.kind} text={msg.text} />
        </div>
      )}
      {assets && assets.length === 0 && !disabled && <p className="text-sm text-zinc-500">No uploads yet.</p>}
      {assets && assets.length > 0 && (
        <table className="w-full text-sm">
          <thead className="text-zinc-400">
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>Size</th>
              <th>URL</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {assets.map((a) => (
              <tr key={a.id} className="border-t border-zinc-800">
                <td>{a.name}</td>
                <td className="text-xs text-zinc-400">{a.contentType}</td>
                <td className="text-xs text-zinc-400">{(a.size / 1024).toFixed(1)} KB</td>
                <td className="font-mono text-xs">{a.url}</td>
                <td className="text-right whitespace-nowrap">
                  <Btn tone="secondary" onClick={() => navigator.clipboard?.writeText(a.url)}>
                    Copy URL
                  </Btn>{" "}
                  {a.url.match(/\.(glb|gltf)$/) && (
                    <Btn tone="secondary" onClick={() => onPick(a.url)}>
                      Use in manifest
                    </Btn>
                  )}{" "}
                  <Btn tone="danger" onClick={() => remove(a)}>
                    Delete
                  </Btn>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Panel>
  );
}

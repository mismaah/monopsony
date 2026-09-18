import { useEffect, useState } from "react";
import { Route, Routes, useNavigate, useParams } from "react-router-dom";
import type { CardDef, Config, SpaceDef } from "@monopsony/protocol";
import { api } from "@/lib/http";
import { Btn, Field, Notice, Panel } from "@/lib/ui";

interface Row {
  id: string;
  version: number;
  name: string;
  published: boolean;
  createdBy: string;
  createdAt: string;
}

export default function ConfigsPage() {
  return (
    <Routes>
      <Route index element={<ConfigList />} />
      <Route path="edit/:id" element={<ConfigEditor />} />
    </Routes>
  );
}

function ConfigList() {
  const [rows, setRows] = useState<Row[]>([]);
  const [msg, setMsg] = useState<string | null>(null);
  const nav = useNavigate();
  const load = () => api<{ configs: Row[] }>("GET", "/admin/api/configs").then((r) => setRows(r.configs));
  useEffect(() => {
    void load();
  }, []);
  return (
    <Panel title="Game configs">
      <p className="text-sm text-zinc-400 mb-3">
        Configs are immutable versions. Publishing one makes it the default for new tables; running games keep the version they started with.
      </p>
      {msg && <Notice kind="ok" text={msg} />}
      <table className="w-full text-sm mt-2">
        <thead className="text-zinc-400">
          <tr>
            <th>Version</th>
            <th>Name</th>
            <th>Created</th>
            <th>Status</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.id} className="border-t border-zinc-800">
              <td>v{r.version}</td>
              <td>{r.name}</td>
              <td className="text-zinc-400">{new Date(r.createdAt).toLocaleString()}</td>
              <td>{r.published ? <span className="text-emerald-300">published</span> : <span className="text-zinc-500">draft</span>}</td>
              <td className="text-right space-x-2">
                <Btn tone="secondary" onClick={() => nav(`edit/${r.id}`)}>
                  New version from this
                </Btn>
                {!r.published && (
                  <Btn
                    onClick={async () => {
                      await api("POST", `/admin/api/configs/${r.id}/publish`);
                      setMsg(`v${r.version} is now the published config.`);
                      await load();
                    }}
                  >
                    Publish
                  </Btn>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </Panel>
  );
}

const SPACE_TYPES = ["go", "street", "railroad", "utility", "chance", "community_chest", "tax", "jail", "free_parking", "go_to_jail"];
const CARD_EFFECTS = ["money", "move_to", "move_back", "move_to_nearest", "go_to_jail", "get_out_of_jail", "repairs", "collect_from_each", "pay_each"];

function ConfigEditor() {
  const { id = "" } = useParams();
  const nav = useNavigate();
  const [cfg, setCfg] = useState<Config | null>(null);
  const [name, setName] = useState("");
  const [tab, setTab] = useState<"rules" | "spaces" | "cards" | "json">("rules");
  const [json, setJson] = useState("");
  const [status, setStatus] = useState<{ kind: "ok" | "err"; text: string } | null>(null);
  const [publish, setPublish] = useState(false);

  useEffect(() => {
    api<{ config: { config: Config; name: string; version: number } }>("GET", `/admin/api/configs/${id}`).then((r) => {
      setCfg(r.config.config);
      setName(`${r.config.name} (copy)`);
      setJson(JSON.stringify(r.config.config, null, 2));
    });
  }, [id]);

  if (!cfg) return <div className="text-zinc-400">Loading…</div>;

  const update = (patch: Partial<Config>) => {
    const next = { ...cfg, ...patch };
    setCfg(next);
    setJson(JSON.stringify(next, null, 2));
  };
  const setSpace = (i: number, patch: Partial<SpaceDef>) => update({ spaces: cfg.spaces.map((s, j) => (j === i ? { ...s, ...patch } : s)) });
  const setCard = (deck: "chance" | "communityChest", i: number, patch: Partial<CardDef>) =>
    update({ [deck]: cfg[deck].map((c, j) => (j === i ? { ...c, ...patch } : c)) } as Partial<Config>);

  async function validate() {
    const res = await api<{ valid: boolean; error?: string }>("POST", "/admin/api/configs/validate", cfg);
    setStatus(res.valid ? { kind: "ok", text: "Config is valid." } : { kind: "err", text: res.error ?? "invalid" });
  }
  async function save() {
    try {
      const res = await api<{ config: { version: number } }>("POST", "/admin/api/configs", { name, config: cfg, publish });
      setStatus({ kind: "ok", text: `Saved as v${res.config.version}${publish ? " and published" : ""}.` });
      setTimeout(() => nav("/configs"), 800);
    } catch (e) {
      setStatus({ kind: "err", text: (e as Error).message });
    }
  }
  function applyJson() {
    try {
      const parsed = JSON.parse(json) as Config;
      setCfg(parsed);
      setStatus({ kind: "ok", text: "JSON applied to the editor." });
    } catch (e) {
      setStatus({ kind: "err", text: `JSON error: ${(e as Error).message}` });
    }
  }

  const rules = cfg.rules;
  const numRule = (k: keyof typeof rules, label: string) => (
    <Field label={label} key={k}>
      <input type="number" value={rules[k] as number} onChange={(e) => update({ rules: { ...rules, [k]: +e.target.value } })} />
    </Field>
  );
  const boolRule = (k: keyof typeof rules, label: string) => (
    <label key={k} className="flex items-center gap-2 text-sm">
      <input type="checkbox" checked={rules[k] as boolean} onChange={(e) => update({ rules: { ...rules, [k]: e.target.checked } })} /> {label}
    </label>
  );

  return (
    <Panel
      title="New config version"
      actions={
        <>
          <label className="text-sm flex items-center gap-1">
            <input type="checkbox" checked={publish} onChange={(e) => setPublish(e.target.checked)} /> publish on save
          </label>
          <Btn tone="secondary" onClick={validate}>Validate</Btn>
          <Btn onClick={save}>Save version</Btn>
        </>
      }
    >
      <div className="grid md:grid-cols-3 gap-3 mb-4">
        <Field label="Version name">
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Game title (shown on the board)">
          <input value={cfg.name} onChange={(e) => update({ name: e.target.value })} />
        </Field>
        <Field label="Currency symbol">
          <input value={cfg.currency} onChange={(e) => update({ currency: e.target.value })} />
        </Field>
      </div>
      {status && <div className="mb-3"><Notice kind={status.kind} text={status.text} /></div>}
      <div className="flex gap-1 mb-3 border-b border-zinc-800">
        {(["rules", "spaces", "cards", "json"] as const).map((t) => (
          <button key={t} onClick={() => setTab(t)} className={`px-3 py-1.5 text-sm capitalize ${tab === t ? "border-b-2 border-emerald-400 text-white" : "text-zinc-400"}`}>
            {t}
          </button>
        ))}
      </div>

      {tab === "rules" && (
        <div className="grid md:grid-cols-4 gap-3">
          {numRule("startCash", "Start cash")}
          {numRule("goSalary", "Go salary")}
          {numRule("jailFine", "Jail fine")}
          {numRule("maxJailTurns", "Max jail turns")}
          {numRule("maxDoubles", "Doubles → jail")}
          {numRule("houseSupply", "House supply")}
          {numRule("hotelSupply", "Hotel supply")}
          {numRule("mortgageInterestPct", "Mortgage interest %")}
          {numRule("buildingSellPct", "Building resale %")}
          {numRule("turnLimit", "Turn limit (0 = none)")}
          <div className="md:col-span-4 grid md:grid-cols-2 gap-2 mt-2">
            {boolRule("auctionsEnabled", "Auction declined properties (standard rule)")}
            {boolRule("freeParkingJackpot", "Free Parking jackpot (house rule)")}
            {boolRule("doubleSalaryOnGoLanding", "Double salary when landing on Go (house rule)")}
            {boolRule("noRentInJail", "No rent collected while in jail (house rule)")}
          </div>
        </div>
      )}

      {tab === "spaces" && (
        <div className="overflow-auto">
          <table className="text-xs w-full">
            <thead className="text-zinc-400">
              <tr>
                <th>#</th>
                <th>Name</th>
                <th>Type</th>
                <th>Group</th>
                <th>Color</th>
                <th>Price</th>
                <th>Rent ×6</th>
                <th>House</th>
                <th>Tax</th>
              </tr>
            </thead>
            <tbody>
              {cfg.spaces.map((s, i) => (
                <tr key={i} className="border-t border-zinc-800">
                  <td>{i}</td>
                  <td><input className="w-40" value={s.name} onChange={(e) => setSpace(i, { name: e.target.value })} /></td>
                  <td>
                    <select value={s.type} onChange={(e) => setSpace(i, { type: e.target.value })}>
                      {SPACE_TYPES.map((t) => <option key={t}>{t}</option>)}
                    </select>
                  </td>
                  <td><input className="w-20" value={s.group ?? ""} onChange={(e) => setSpace(i, { group: e.target.value })} /></td>
                  <td><input type="color" value={s.color || "#888888"} onChange={(e) => setSpace(i, { color: e.target.value })} /></td>
                  <td><input className="w-16" type="number" value={s.price ?? 0} onChange={(e) => setSpace(i, { price: +e.target.value })} /></td>
                  <td>
                    <div className="flex gap-0.5">
                      {(s.rent ?? [0, 0, 0, 0, 0, 0]).map((r, k) => (
                        <input key={k} className="w-16" type="number" value={r} onChange={(e) => { const rent = [...(s.rent ?? [0, 0, 0, 0, 0, 0])]; rent[k] = +e.target.value; setSpace(i, { rent }); }} />
                      ))}
                    </div>
                  </td>
                  <td><input className="w-16" type="number" value={s.houseCost ?? 0} onChange={(e) => setSpace(i, { houseCost: +e.target.value })} /></td>
                  <td><input className="w-16" type="number" value={s.taxAmount ?? 0} onChange={(e) => setSpace(i, { taxAmount: +e.target.value })} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {tab === "cards" && (
        <div className="grid md:grid-cols-2 gap-6">
          {(["chance", "communityChest"] as const).map((deck) => (
            <div key={deck}>
              <div className="flex items-center mb-2">
                <h3 className="font-medium">
                  {cfg.spaces.find((s) => s.type === (deck === "chance" ? "chance" : "community_chest"))?.name ?? deck}
                  <span className="ml-2 text-xs text-zinc-500 font-normal">{deck === "chance" ? "chance deck" : "community deck"}</span>
                </h3>
                <div className="flex-1" />
                <Btn tone="secondary" onClick={() => update({ [deck]: [...cfg[deck], { id: `${deck.slice(0, 2)}${Date.now() % 100000}`, text: "New card", effect: "money", amount: 0 }] } as Partial<Config>)}>
                  + card
                </Btn>
              </div>
              <div className="space-y-2">
                {cfg[deck].map((c, i) => (
                  <div key={c.id} className="border border-zinc-800 rounded p-2 text-xs space-y-1">
                    <div className="flex gap-1">
                      <input className="w-20" value={c.id} onChange={(e) => setCard(deck, i, { id: e.target.value })} />
                      <select value={c.effect} onChange={(e) => setCard(deck, i, { effect: e.target.value })}>
                        {CARD_EFFECTS.map((t) => <option key={t}>{t}</option>)}
                      </select>
                      <button className="text-rose-400 ml-auto" onClick={() => update({ [deck]: cfg[deck].filter((_, j) => j !== i) } as Partial<Config>)}>remove</button>
                    </div>
                    <input className="w-full" value={c.text} onChange={(e) => setCard(deck, i, { text: e.target.value })} />
                    <div className="flex gap-1 flex-wrap">
                      {["amount", "target", "steps", "perHouse", "perHotel"].map((k) => (
                        <label key={k} className="flex items-center gap-1">
                          {k}
                          <input className="w-14" type="number" value={(c as unknown as Record<string, number>)[k] ?? 0} onChange={(e) => setCard(deck, i, { [k]: +e.target.value } as Partial<CardDef>)} />
                        </label>
                      ))}
                      <label className="flex items-center gap-1">
                        group <input className="w-20" value={c.group ?? ""} onChange={(e) => setCard(deck, i, { group: e.target.value })} />
                      </label>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}

      {tab === "json" && (
        <div className="space-y-2">
          <textarea className="w-full h-[60vh] font-mono text-xs" value={json} onChange={(e) => setJson(e.target.value)} />
          <Btn tone="secondary" onClick={applyJson}>Apply JSON</Btn>
        </div>
      )}
    </Panel>
  );
}

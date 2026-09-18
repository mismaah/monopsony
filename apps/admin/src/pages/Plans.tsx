import { useEffect, useState } from "react";
import { api } from "@/lib/http";
import { Btn, Field, Notice, Panel } from "@/lib/ui";

interface Caps {
  tier: string;
  maxPlayersPerRoom: number;
  privateRooms: boolean;
  houseRules: boolean;
  showAds: boolean;
  statsHistoryDays: number;
  maxConcurrentGames: number;
  cosmeticSlots: string[];
}
interface Plan {
  tier: string;
  priceId?: string;
  caps: Caps;
}

const SLOTS = ["token", "board", "dice", "buildings", "cards"];

export default function PlansPage() {
  const [plans, setPlans] = useState<Plan[]>([]);
  const [msg, setMsg] = useState<{ kind: "ok" | "err"; text: string } | null>(null);
  useEffect(() => {
    api<{ plans: Plan[] }>("GET", "/admin/api/plans").then((r) => setPlans(r.plans.sort((a, b) => a.tier.localeCompare(b.tier))));
  }, []);

  const set = (tier: string, patch: Partial<Plan> | { caps: Partial<Caps> }) =>
    setPlans(plans.map((p) => (p.tier === tier ? { ...p, ...patch, caps: { ...p.caps, ...("caps" in patch ? patch.caps : {}) } } : p)));

  async function save(p: Plan) {
    try {
      await api("PUT", `/admin/api/plans/${p.tier}`, p);
      setMsg({ kind: "ok", text: `${p.tier} plan saved — applies to new lobby actions immediately.` });
    } catch (e) {
      setMsg({ kind: "err", text: (e as Error).message });
    }
  }

  return (
    <Panel title="Plans & tiers">
      {msg && <div className="mb-3"><Notice kind={msg.kind} text={msg.text} /></div>}
      <div className="grid md:grid-cols-2 gap-4">
        {plans.map((p) => (
          <div key={p.tier} className="border border-zinc-800 rounded-lg p-4 space-y-3">
            <h3 className="font-semibold capitalize">{p.tier}</h3>
            {p.tier !== "free" && (
              <Field label="Payment provider price id (Stripe price_…)">
                <input value={p.priceId ?? ""} onChange={(e) => set(p.tier, { priceId: e.target.value })} placeholder="price_123" />
              </Field>
            )}
            <div className="grid grid-cols-2 gap-3">
              <Field label="Max players per room">
                <input type="number" min={2} max={8} value={p.caps.maxPlayersPerRoom} onChange={(e) => set(p.tier, { caps: { maxPlayersPerRoom: +e.target.value } })} />
              </Field>
              <Field label="Max concurrent games (0 = ∞)">
                <input type="number" min={0} value={p.caps.maxConcurrentGames} onChange={(e) => set(p.tier, { caps: { maxConcurrentGames: +e.target.value } })} />
              </Field>
              <Field label="Stats history days (0 = ∞)">
                <input type="number" min={0} value={p.caps.statsHistoryDays} onChange={(e) => set(p.tier, { caps: { statsHistoryDays: +e.target.value } })} />
              </Field>
            </div>
            <div className="grid grid-cols-2 gap-2 text-sm">
              {(
                [
                  ["privateRooms", "Private rooms"],
                  ["houseRules", "House rules"],
                  ["showAds", "Show ads"],
                ] as const
              ).map(([k, label]) => (
                <label key={k} className="flex items-center gap-2">
                  <input type="checkbox" checked={p.caps[k]} onChange={(e) => set(p.tier, { caps: { [k]: e.target.checked } })} /> {label}
                </label>
              ))}
            </div>
            <div>
              <div className="text-xs uppercase text-zinc-400 mb-1">Cosmetic slots</div>
              <div className="flex gap-3 text-sm flex-wrap">
                {SLOTS.map((s) => (
                  <label key={s} className="flex items-center gap-1">
                    <input
                      type="checkbox"
                      checked={p.caps.cosmeticSlots.includes(s)}
                      onChange={(e) => set(p.tier, { caps: { cosmeticSlots: e.target.checked ? [...p.caps.cosmeticSlots, s] : p.caps.cosmeticSlots.filter((x) => x !== s) } })}
                    />
                    {s}
                  </label>
                ))}
              </div>
            </div>
            <Btn onClick={() => save(p)}>Save {p.tier}</Btn>
          </div>
        ))}
      </div>
    </Panel>
  );
}

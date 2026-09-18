import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api } from "@/api/http";
import { useAuth } from "@/store/auth";
import { Button, Card } from "@/lib/ui";

interface PlanCaps {
  maxPlayersPerRoom: number;
  privateRooms: boolean;
  houseRules: boolean;
  showAds: boolean;
  statsHistoryDays: number;
  // Go serialises a nil slice as null, so tolerate a missing list.
  cosmeticSlots: string[] | null;
}

interface Billing {
  subscription: { status: string; tier: string; periodEnd: string } | null;
  purchases: { id: string; sku: string; amountCents: number; currency: string; createdAt: string }[];
  plans: { tier: string; caps: PlanCaps }[];
  provider: string;
}

export default function AccountPage() {
  const { user, caps, reloadMe } = useAuth();
  const [params] = useSearchParams();
  const [info, setInfo] = useState<Billing | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    void reloadMe();
    void api<Billing>("GET", "/api/me/billing").then(setInfo).catch((e) => setError(e.message));
  }, [reloadMe]);

  async function go(path: string, body?: unknown) {
    setError(null);
    try {
      const res = await api<{ url: string }>("POST", path, body);
      location.href = res.url;
    } catch (e) {
      setError((e as Error).message);
    }
  }

  const premium = caps?.tier === "premium";
  return (
    <div className="max-w-3xl mx-auto p-4 space-y-4">
      {params.get("upgraded") && <div className="bg-emerald-500/15 border border-emerald-500/40 rounded-lg px-4 py-2 text-sm">Welcome to premium!</div>}
      <Card title="Account">
        <div className="text-sm space-y-1">
          <div>
            <span className="text-slate-400">Name</span> {user?.name}
          </div>
          <div>
            <span className="text-slate-400">Email</span> {user?.email ?? <span className="text-slate-500">guest account — progress is lost when your session expires</span>}
          </div>
          <div>
            <span className="text-slate-400">Plan</span> <span className={premium ? "text-amber-300" : ""}>{caps?.tier}</span>
          </div>
        </div>
      </Card>
      <Card title="Plan">
        {info && (
          <div className="grid md:grid-cols-2 gap-3 text-sm">
            {info.plans
              .sort((a, b) => (a.tier === "free" ? -1 : 1) - (b.tier === "free" ? -1 : 1))
              .map((p) => (
                <div key={p.tier} className={`rounded-lg border p-3 ${p.tier === caps?.tier ? "border-emerald-400" : "border-slate-800"}`}>
                  <div className="font-semibold capitalize mb-1">{p.tier}</div>
                  <ul className="text-slate-300 space-y-0.5">
                    <li>Up to {p.caps.maxPlayersPerRoom} players per table</li>
                    <li>{p.caps.privateRooms ? "Private tables with invite codes" : "Public tables only"}</li>
                    <li>{p.caps.houseRules ? "House rules" : "Standard rules only"}</li>
                    <li>{p.caps.showAds ? "Ad-supported" : "No ads"}</li>
                    <li>{p.caps.statsHistoryDays === 0 ? "Full match history" : `${p.caps.statsHistoryDays}-day match history`}</li>
                    <li>Cosmetic slots: {p.caps.cosmeticSlots?.length ? p.caps.cosmeticSlots.join(", ") : "none"}</li>
                  </ul>
                </div>
              ))}
          </div>
        )}
        <div className="flex gap-2 mt-3">
          {!premium && (
            <Button onClick={() => go("/api/billing/subscribe", { tier: "premium" })} disabled={user?.guest}>
              Upgrade to premium
            </Button>
          )}
          {info?.subscription && (
            <Button variant="secondary" onClick={() => go("/api/billing/portal")}>
              Manage subscription
            </Button>
          )}
        </div>
        {info?.subscription && (
          <p className="text-xs text-slate-400 mt-2">
            Status: {info.subscription.status}
            {info.subscription.periodEnd && ` · renews ${new Date(info.subscription.periodEnd).toLocaleDateString()}`}
          </p>
        )}
      </Card>
      <Card title="Purchases">
        {info?.purchases.length === 0 && <p className="text-sm text-slate-500">No purchases yet.</p>}
        <ul className="text-sm divide-y divide-slate-800">
          {info?.purchases.map((p) => (
            <li key={p.id} className="py-1.5 flex justify-between">
              <span>{p.sku}</span>
              <span className="text-slate-400">
                ${(p.amountCents / 100).toFixed(2)} · {new Date(p.createdAt).toLocaleDateString()}
              </span>
            </li>
          ))}
        </ul>
      </Card>
      {error && <p className="text-rose-400 text-sm">{error}</p>}
    </div>
  );
}

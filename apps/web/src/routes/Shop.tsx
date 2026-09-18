import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api } from "@/api/http";
import { useAuth } from "@/store/auth";
import { Button, Card } from "@/lib/ui";
import { TokenPreview } from "@/game3d/Preview";
import { useCosmetics } from "@/store/cosmetics";
import type { Manifest } from "@/game3d/skins";
import { AdSlot } from "@/hud/AdSlot";

interface Item {
  id: string;
  slot: string;
  name: string;
  description: string;
  priceCents: number;
  currency: string;
  tierRequired?: string;
  manifest: Manifest;
  owned: boolean;
  equipped: boolean;
  locked?: string;
}

const slotLabels: Record<string, string> = { token: "Tokens", board: "Boards", dice: "Dice", buildings: "Buildings", cards: "Cards" };

export default function ShopPage() {
  const { user, caps, reloadMe } = useAuth();
  const reloadManifests = useCosmetics((s) => s.load);
  const [params] = useSearchParams();
  const [items, setItems] = useState<Item[]>([]);
  const [slots, setSlots] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  async function load() {
    const res = await api<{ items: Item[]; slots: string[] }>("GET", "/api/shop/catalog");
    setItems(res.items ?? []);
    setSlots(res.slots ?? []);
    void reloadManifests(true);
  }
  useEffect(() => {
    void load();
    void reloadMe();
  }, [reloadMe]);

  async function equip(item: Item) {
    setError(null);
    setBusy(item.id);
    try {
      await api("PUT", "/api/me/loadout", { slot: item.slot, itemId: item.equipped ? "" : item.id });
      await load();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(null);
    }
  }

  async function buy(item: Item) {
    setError(null);
    setBusy(item.id);
    try {
      const res = await api<{ url: string }>("POST", "/api/shop/checkout", { itemId: item.id });
      location.href = res.url;
    } catch (e) {
      setError((e as Error).message);
      setBusy(null);
    }
  }

  async function upgrade() {
    setError(null);
    try {
      const res = await api<{ url: string }>("POST", "/api/billing/subscribe", { tier: "premium" });
      location.href = res.url;
    } catch (e) {
      setError((e as Error).message);
    }
  }

  const purchased = params.get("purchased");

  return (
    <div className="max-w-5xl mx-auto p-4 space-y-4">
      {purchased && <div className="bg-emerald-500/15 border border-emerald-500/40 rounded-lg px-4 py-2 text-sm">Thanks! Your purchase is in your collection.</div>}
      {caps?.tier !== "premium" && (
        <Card>
          <div className="flex items-center gap-4">
            <div className="flex-1">
              <h2 className="font-semibold text-lg">Go premium</h2>
              <p className="text-sm text-slate-400">Private tables, up to 8 players, house rules, every cosmetic slot, no ads, full match history.</p>
            </div>
            <Button onClick={upgrade} disabled={user?.guest}>
              {user?.guest ? "Create an account first" : "Upgrade"}
            </Button>
          </div>
        </Card>
      )}
      {error && <p className="text-rose-400 text-sm">{error}</p>}
      <AdSlot placement="shop" />
      {slots.map((slot) => {
        const list = items.filter((i) => i.slot === slot);
        if (list.length === 0) return null;
        const allowed = caps?.cosmeticSlots.includes(slot);
        return (
          <Card key={slot} title={`${slotLabels[slot] ?? slot}${allowed ? "" : " · premium slot"}`}>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
              {list.map((item) => (
                <div key={item.id} className={`rounded-lg border p-3 flex flex-col gap-2 ${item.equipped ? "border-emerald-400" : "border-slate-800"} ${item.locked === "tier" || item.locked === "slot" ? "opacity-60" : ""}`}>
                  <div className="h-24 rounded bg-slate-950/60">
                    {slot === "token" && <TokenPreview itemId={item.id} manifest={item.manifest} color={item.manifest.color} />}
                    {slot !== "token" && <div className="h-full grid place-items-center text-3xl">{slot === "dice" ? "🎲" : slot === "board" ? "🗺️" : "🏠"}</div>}
                  </div>
                  <div>
                    <div className="font-medium text-sm">{item.name}</div>
                    {item.description && <div className="text-xs text-slate-400">{item.description}</div>}
                  </div>
                  <div className="mt-auto flex items-center gap-2">
                    <span className="text-xs text-slate-400">
                      {item.tierRequired ? "premium" : item.priceCents ? `$${(item.priceCents / 100).toFixed(2)}` : "free"}
                    </span>
                    <span className="flex-1" />
                    {item.locked === "purchase" && (
                      <Button disabled={busy === item.id || user?.guest} onClick={() => buy(item)}>
                        Buy
                      </Button>
                    )}
                    {item.locked === "tier" && <span className="text-xs text-amber-300">premium</span>}
                    {item.locked === "slot" && <span className="text-xs text-amber-300">premium slot</span>}
                    {!item.locked && (
                      <Button variant={item.equipped ? "secondary" : "primary"} disabled={busy === item.id} onClick={() => equip(item)}>
                        {item.equipped ? "Equipped" : "Equip"}
                      </Button>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </Card>
        );
      })}
    </div>
  );
}

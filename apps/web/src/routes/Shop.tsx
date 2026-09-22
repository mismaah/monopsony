import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api } from "@/api/http";
import { useAuth } from "@/store/auth";
import { Button, Card } from "@/lib/ui";
import { useCosmetics } from "@/store/cosmetics";
import { AdSlot } from "@/hud/AdSlot";
import { CosmeticCard, EquipButton, slotLabels, type CosmeticItem as Item } from "@/hud/CosmeticCard";
import { PreviewStage } from "@monopsony/cosmetics";

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
    <PreviewStage className="max-w-5xl mx-auto p-4 space-y-4">
      {purchased && (
        <div className="bg-emerald-500/15 border border-emerald-500/40 rounded-lg px-4 py-2 text-sm">
          Thanks! Your purchase is in{" "}
          <Link to="/collection" className="underline hover:text-emerald-200">
            your collection
          </Link>
          .
        </div>
      )}
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
        const allowed = caps?.cosmeticSlots?.includes(slot) ?? false;
        return (
          <Card key={slot} title={`${slotLabels[slot] ?? slot}${allowed ? "" : " · premium slot"}`}>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
              {list.map((item) => (
                <CosmeticCard
                  key={item.id}
                  item={item}
                  footer={
                    <>
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
                      {!item.locked && <EquipButton item={item} busy={busy === item.id} onEquip={equip} />}
                    </>
                  }
                />
              ))}
            </div>
          </Card>
        );
      })}
    </PreviewStage>
  );
}

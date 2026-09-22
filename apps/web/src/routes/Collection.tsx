import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/api/http";
import { useAuth } from "@/store/auth";
import { Button, Card } from "@/lib/ui";
import { useCosmetics } from "@/store/cosmetics";
import { CosmeticCard, EquipButton, slotLabels, type CosmeticItem } from "@/hud/CosmeticCard";
import { PreviewStage } from "@monopsony/cosmetics";

/**
 * Everything the player owns (bought, free, or unlocked by their plan),
 * grouped by slot, with the equipped item highlighted. Uses the same catalog
 * endpoint as the shop and simply hides anything they cannot equip.
 */
export default function CollectionPage() {
  const { caps, reloadMe } = useAuth();
  const reloadManifests = useCosmetics((s) => s.load);
  const [items, setItems] = useState<CosmeticItem[]>([]);
  const [slots, setSlots] = useState<string[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  async function load() {
    const res = await api<{ items: CosmeticItem[]; slots: string[] }>("GET", "/api/shop/catalog");
    setItems(res.items ?? []);
    setSlots(res.slots ?? []);
    setLoaded(true);
    void reloadManifests(true);
  }
  useEffect(() => {
    void load().catch((e) => setError((e as Error).message));
    void reloadMe();
  }, [reloadMe]);

  async function setSlot(slot: string, itemId: string) {
    setError(null);
    setBusy(itemId || slot);
    try {
      await api("PUT", "/api/me/loadout", { slot, itemId });
      await load();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(null);
    }
  }

  // Owned items the plan lets the player use; "slot"-locked ones are still
  // theirs, so keep them visible but explain why they cannot be equipped.
  const owned = items.filter((i) => i.owned && i.locked !== "tier");
  const total = owned.length;

  return (
    <PreviewStage className="max-w-5xl mx-auto p-4 space-y-4">
      <div className="flex items-baseline gap-3">
        <h1 className="text-xl font-semibold">Your collection</h1>
        <span className="text-sm text-slate-400">
          {total} item{total === 1 ? "" : "s"}
        </span>
        <span className="flex-1" />
        <Link to="/shop" className="text-sm text-emerald-300 hover:text-emerald-200">
          Browse the shop →
        </Link>
      </div>
      {error && <p className="text-rose-400 text-sm">{error}</p>}
      {loaded && total === 0 && (
        <Card>
          <p className="text-sm text-slate-400">
            Nothing here yet. Free and purchased items show up here so you can pick what to use at the table.{" "}
            <Link to="/shop" className="text-emerald-300 hover:text-emerald-200">
              Visit the shop
            </Link>
            .
          </p>
        </Card>
      )}
      {slots.map((slot) => {
        const list = owned.filter((i) => i.slot === slot);
        if (list.length === 0) return null;
        const allowed = caps?.cosmeticSlots?.includes(slot) ?? false;
        const equipped = list.find((i) => i.equipped);
        return (
          <Card
            key={slot}
            title={
              <span className="flex items-center gap-3">
                {slotLabels[slot] ?? slot}
                {!allowed && <span className="text-amber-300 normal-case tracking-normal">premium slot</span>}
                <span className="flex-1" />
                {equipped && (
                  <Button variant="ghost" className="normal-case tracking-normal text-xs" disabled={busy === slot} onClick={() => setSlot(slot, "")}>
                    Use default
                  </Button>
                )}
              </span>
            }
          >
            <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
              {list.map((item) => (
                <CosmeticCard
                  key={item.id}
                  item={item}
                  footer={
                    <>
                      <span className="text-xs text-slate-400">{item.tierRequired ? "premium" : item.priceCents ? "purchased" : "free"}</span>
                      <span className="flex-1" />
                      {item.locked === "slot" && <span className="text-xs text-amber-300">premium slot</span>}
                      {!item.locked && <EquipButton item={item} busy={busy === item.id} onEquip={(it) => setSlot(it.slot, it.equipped ? "" : it.id)} />}
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

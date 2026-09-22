import { Button } from "@/lib/ui";
import { CosmeticPreview as Preview3D, type Manifest } from "@monopsony/cosmetics";

/** A catalog entry decorated with the viewer's relationship to it (mirrors cosmetics.Item). */
export interface CosmeticItem {
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

export const slotLabels: Record<string, string> = { token: "Tokens", board: "Boards", dice: "Dice", buildings: "Buildings", cards: "Cards" };

/** Live 3D look of the item; the page must sit inside a PreviewStage. */
export function CosmeticPreview({ item }: { item: CosmeticItem }) {
  return <Preview3D slot={item.slot} itemId={item.id} manifest={item.manifest} className="h-24 rounded bg-slate-950/60" />;
}

/**
 * One shop/collection card. The footer is left to the caller so the shop can
 * show prices and Buy buttons while the collection only shows Equip.
 */
export function CosmeticCard({ item, footer }: { item: CosmeticItem; footer: React.ReactNode }) {
  return (
    <div className={`rounded-lg border p-3 flex flex-col gap-2 ${item.equipped ? "border-emerald-400" : "border-slate-800"} ${item.locked === "tier" || item.locked === "slot" ? "opacity-60" : ""}`}>
      <CosmeticPreview item={item} />
      <div>
        <div className="font-medium text-sm">{item.name}</div>
        {item.description && <div className="text-xs text-slate-400">{item.description}</div>}
      </div>
      <div className="mt-auto flex items-center gap-2">{footer}</div>
    </div>
  );
}

export function EquipButton({ item, busy, onEquip }: { item: CosmeticItem; busy: boolean; onEquip: (item: CosmeticItem) => void }) {
  return (
    <Button variant={item.equipped ? "secondary" : "primary"} disabled={busy} onClick={() => onEquip(item)}>
      {item.equipped ? "Equipped" : "Equip"}
    </Button>
  );
}

import { useGame, send } from "@/store/game";
import { useAuth } from "@/store/auth";
import { Button } from "@/lib/ui";

/**
 * One-stop panel for a debtor: every property they own with the sell/mortgage
 * moves the engine currently allows on it, plus a running tally of how far
 * they are from covering the debt. The server settles the debt itself as soon
 * as cash covers it, at which point the phase changes and the parent closes us.
 */
export function RaiseFundsDialog({ onClose, onTrade }: { onClose: () => void; onTrade: () => void }) {
  const { state, config, legal, animating } = useGame();
  const me = useAuth((s) => s.user);
  if (!state || !config || !me) return null;

  const meP = state.players.find((p) => p.id === me.id);
  if (!meP) return null;
  const owed = (state.debts ?? []).filter((d) => d.debtorId === me.id).reduce((n, d) => n + d.amount, 0);
  const creditors = (state.debts ?? [])
    .filter((d) => d.debtorId === me.id)
    .map((d) => (d.creditorId ? state.players.find((p) => p.id === d.creditorId)?.name ?? d.creditorId : "the bank"));
  const shortfall = Math.max(0, owed - meP.cash);
  const pct = owed > 0 ? Math.min(100, Math.round((meP.cash / owed) * 100)) : 100;

  const can = (t: string, space: number) => legal.some((a) => a.type === t && a.space === space);
  const sellPct = config.rules.buildingSellPct;
  const mine = state.spaces.map((s, i) => ({ s, i })).filter((x) => x.s.ownerId === me.id);
  // Most cash still raisable without a trade, so the player can see at a glance
  // whether selling and mortgaging alone can get them out.
  const raisable = mine.reduce((n, { s, i }) => {
    const def = config.spaces[i];
    return n + (s.mortgaged ? 0 : Math.floor((def.price ?? 0) / 2)) + Math.floor((s.houses * (def.houseCost ?? 0) * sellPct) / 100);
  }, 0);
  const canCover = meP.cash + raisable >= owed;

  return (
    <div className="fixed inset-0 bg-black/60 z-30 flex items-end sm:items-center justify-center overflow-y-auto overscroll-contain" onClick={onClose}>
      <div
        className="bg-slate-900 border border-slate-700 w-full sm:w-[560px] sm:max-w-[95vw] rounded-t-2xl sm:rounded-xl p-4 max-h-[92dvh] sm:max-h-[90vh] overflow-y-auto pad-safe-bottom"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 mb-1">
          <h3 className="font-semibold">Raise ${owed.toLocaleString()}</h3>
          <span className="text-xs text-slate-400">owed to {creditors.join(", ")}</span>
          <span className="flex-1" />
          <span className="text-sm tabular-nums">
            <span className="text-slate-400">Cash </span>
            <span className={shortfall > 0 ? "text-rose-400" : "text-emerald-400"}>${meP.cash.toLocaleString()}</span>
          </span>
        </div>
        <div className="h-1.5 rounded bg-slate-800 overflow-hidden mb-1">
          <div className={`h-full transition-all ${shortfall > 0 ? "bg-amber-400" : "bg-emerald-400"}`} style={{ width: `${pct}%` }} />
        </div>
        <div className="text-xs text-slate-400 mb-3">
          {shortfall > 0 ? (
            <>
              <span className="text-amber-300">${shortfall.toLocaleString()} short.</span>{" "}
              {canCover ? `Selling and mortgaging everything would raise $${raisable.toLocaleString()}.` : `Selling and mortgaging everything only raises $${raisable.toLocaleString()} — you'll need a trade or must declare bankruptcy.`}
            </>
          ) : (
            "Covered — settling…"
          )}
        </div>

        <ul className="space-y-1.5 max-h-[45vh] sm:max-h-72 overflow-auto pr-1">
          {mine.length === 0 && <li className="text-slate-500 text-xs">You own no properties.</li>}
          {mine.map(({ s, i }) => {
            const def = config.spaces[i];
            const mortgageValue = Math.floor((def.price ?? 0) / 2);
            // A hotel that can't be broken into houses (bank shortage) sells whole.
            const perBuilding = Math.floor(((def.houseCost ?? 0) * sellPct) / 100);
            const houseRefund = s.houses === 5 && state.bank.houses < 4 ? perBuilding * 5 : perBuilding;
            const sellable = can("SellHouse", i);
            const mortgageable = can("Mortgage", i);
            const idle = !sellable && !mortgageable;
            return (
              <li key={i} className={`flex flex-wrap items-center gap-x-2 gap-y-1 text-sm rounded px-2 py-1.5 bg-slate-800/50 ${idle ? "opacity-50" : ""}`}>
                <span className="w-2.5 h-2.5 rounded-sm shrink-0" style={{ background: def.color || "#64748b" }} />
                <span className="flex-1 min-w-[6rem] truncate">{def.name}</span>
                {s.houses > 0 && <span className="text-xs text-slate-400">{s.houses === 5 ? "hotel" : `${s.houses} house${s.houses > 1 ? "s" : ""}`}</span>}
                {s.mortgaged && <span className="text-xs text-amber-300">mortgaged</span>}
                {sellable && (
                  <Button variant="secondary" className="py-1" disabled={animating} onClick={() => send("SellHouse", { space: i })}>
                    Sell building +${houseRefund}
                  </Button>
                )}
                {mortgageable && (
                  <Button variant="secondary" className="py-1" disabled={animating} onClick={() => send("Mortgage", { space: i })}>
                    Mortgage +${mortgageValue}
                  </Button>
                )}
                {idle && !s.mortgaged && (
                  <span className="text-[11px] text-slate-500">{s.houses > 0 ? "sell evenly across the set" : "sell buildings in the set first"}</span>
                )}
              </li>
            );
          })}
        </ul>

        <div className="flex flex-wrap items-center gap-2 mt-4 sticky bottom-0 bg-slate-900 pt-2">
          {legal.some((a) => a.type === "DeclareBankruptcy") && (
            <Button variant="danger" disabled={animating} onClick={() => send("DeclareBankruptcy")}>Declare bankruptcy</Button>
          )}
          <span className="flex-1" />
          {legal.some((a) => a.type === "ProposeTrade") && (
            <Button variant="secondary" disabled={animating} onClick={onTrade}>Trade…</Button>
          )}
          <Button variant="ghost" onClick={onClose}>Hide</Button>
        </div>
      </div>
    </div>
  );
}

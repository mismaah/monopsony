import { useGame, send } from "@/store/game";
import { Button, seatColors } from "@/lib/ui";

/** Deed card for the selected space, with the actions the engine allows on it. */
export function PropertyPanel() {
  const { state, config, legal, selectedSpace, select } = useGame();
  if (!state || !config || selectedSpace === null) return null;
  const def = config.spaces[selectedSpace];
  const rent = def.rent ?? [0, 0, 0, 0, 0, 0];
  const price = def.price ?? 0;
  const st = state.spaces[selectedSpace];
  const ownerIdx = st.ownerId ? state.players.findIndex((p) => p.id === st.ownerId) : -1;
  const owner = ownerIdx >= 0 ? state.players[ownerIdx] : null;
  const can = (t: string) => legal.some((a) => a.type === t && a.space === selectedSpace);
  const ownable = def.type === "street" || def.type === "railroad" || def.type === "utility";

  return (
    <div className="w-full sm:w-64 bg-slate-900/90 backdrop-blur-sm border border-slate-800 rounded-xl overflow-hidden flex flex-col max-h-[46vh] sm:max-h-none">
      <div className="px-3 py-2 text-center font-semibold" style={{ background: def.color || "#334155", color: "#fff" }}>
        {def.name}
      </div>
      <div className="p-3 text-sm overflow-y-auto grid grid-cols-2 gap-x-4 gap-y-1 sm:block sm:space-y-1.5">
        {ownable && (
          <div className="flex justify-between">
            <span className="text-slate-400">Price</span>
            <span>${price}</span>
          </div>
        )}
        {def.type === "street" && (
          <>
            <Row label="Rent" v={`$${rent[0]}`} />
            <Row label="With colour set" v={`$${rent[0] * 2}`} />
            <Row label="1 house" v={`$${rent[1]}`} />
            <Row label="2 houses" v={`$${rent[2]}`} />
            <Row label="3 houses" v={`$${rent[3]}`} />
            <Row label="4 houses" v={`$${rent[4]}`} />
            <Row label="Hotel" v={`$${rent[5]}`} />
            <Row label="House cost" v={`$${def.houseCost ?? 0}`} />
          </>
        )}
        {def.type === "railroad" && (
          <>
            <Row label="1 owned" v={`$${rent[0]}`} />
            <Row label="2 owned" v={`$${rent[1]}`} />
            <Row label="3 owned" v={`$${rent[2]}`} />
            <Row label="4 owned" v={`$${rent[3]}`} />
          </>
        )}
        {def.type === "utility" && (
          <>
            <Row label="1 owned" v={`${rent[0]}× dice`} />
            <Row label="2 owned" v={`${rent[1]}× dice`} />
          </>
        )}
        {def.type === "tax" && <Row label="Pay" v={`$${def.taxAmount}`} />}
        {ownable && <Row label="Mortgage value" v={`$${Math.floor(price / 2)}`} />}
        {owner && (
          <div className="col-span-2 flex items-center gap-2 pt-1 mt-1 border-t border-slate-800">
            <span className="w-2 h-2 rounded-full" style={{ background: seatColors[ownerIdx % seatColors.length] }} />
            <span>{owner.name}</span>
            {st.mortgaged && <span className="text-amber-300 text-xs ml-auto">mortgaged</span>}
            {st.houses > 0 && <span className="text-xs ml-auto">{st.houses === 5 ? "hotel" : `${st.houses} house${st.houses > 1 ? "s" : ""}`}</span>}
          </div>
        )}
        <div className="col-span-2 flex flex-wrap gap-1.5 pt-2">
          {can("BuildHouse") && <Button onClick={() => send("BuildHouse", { space: selectedSpace })}>Build ${def.houseCost ?? 0}</Button>}
          {can("SellHouse") && <Button variant="secondary" onClick={() => send("SellHouse", { space: selectedSpace })}>Sell building</Button>}
          {can("Mortgage") && <Button variant="secondary" onClick={() => send("Mortgage", { space: selectedSpace })}>Mortgage</Button>}
          {can("Unmortgage") && (
            <Button variant="secondary" onClick={() => send("Unmortgage", { space: selectedSpace })}>
              Unmortgage ${Math.floor(price / 2) + Math.floor((Math.floor(price / 2) * config.rules.mortgageInterestPct) / 100)}
            </Button>
          )}
          <Button variant="ghost" onClick={() => select(null)}>Close</Button>
        </div>
      </div>
    </div>
  );
}

function Row({ label, v }: { label: string; v: string }) {
  return (
    <div className="flex justify-between">
      <span className="text-slate-400">{label}</span>
      <span>{v}</span>
    </div>
  );
}

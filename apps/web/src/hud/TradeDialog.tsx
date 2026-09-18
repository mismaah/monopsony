import { useMemo, useState } from "react";
import { useGame, send } from "@/store/game";
import { useAuth } from "@/store/auth";
import { Button } from "@/lib/ui";

/** Two-column trade builder: what I give, what I get. */
export function TradeDialog({ onClose }: { onClose: () => void }) {
  const { state, config } = useGame();
  const me = useAuth((s) => s.user);
  const others = useMemo(() => state?.players.filter((p) => !p.bankrupt && p.id !== me?.id) ?? [], [state, me]);
  const [toId, setToId] = useState(others[0]?.id ?? "");
  const [giveCash, setGiveCash] = useState(0);
  const [getCash, setGetCash] = useState(0);
  const [giveProps, setGiveProps] = useState<number[]>([]);
  const [getProps, setGetProps] = useState<number[]>([]);
  const [giveCards, setGiveCards] = useState(0);
  const [getCards, setGetCards] = useState(0);
  if (!state || !config || !me) return null;

  const mine = state.spaces.map((s, i) => ({ s, i })).filter((x) => x.s.ownerId === me.id);
  const theirs = state.spaces.map((s, i) => ({ s, i })).filter((x) => x.s.ownerId === toId);
  const meP = state.players.find((p) => p.id === me.id)!;
  const themP = state.players.find((p) => p.id === toId);
  const toggle = (list: number[], set: (l: number[]) => void, i: number) => set(list.includes(i) ? list.filter((x) => x !== i) : [...list, i]);
  const hasBuildings = (i: number) => {
    const g = config.spaces[i].group;
    return !!g && config.spaces.some((d, j) => d.group === g && state.spaces[j].houses > 0);
  };

  async function propose() {
    await send("ProposeTrade", {
      toId,
      give: { cash: giveCash, properties: giveProps, jailCards: giveCards },
      receive: { cash: getCash, properties: getProps, jailCards: getCards },
    });
    onClose();
  }

  const List = ({ items, chosen, onToggle }: { items: { s: { mortgaged: boolean }; i: number }[]; chosen: number[]; onToggle: (i: number) => void }) => (
    <ul className="space-y-1 max-h-56 overflow-auto pr-1">
      {items.length === 0 && <li className="text-slate-500 text-xs">No properties</li>}
      {items.map(({ s, i }) => {
        const blocked = hasBuildings(i);
        return (
          <li key={i}>
            <label className={`flex items-center gap-2 text-sm ${blocked ? "opacity-40" : ""}`}>
              <input type="checkbox" disabled={blocked} checked={chosen.includes(i)} onChange={() => onToggle(i)} />
              <span className="w-2 h-2 rounded-sm" style={{ background: config.spaces[i].color || "#64748b" }} />
              <span className="flex-1 truncate">{config.spaces[i].name}</span>
              {s.mortgaged && <span className="text-[10px] text-amber-300">M</span>}
            </label>
          </li>
        );
      })}
    </ul>
  );

  return (
    <div className="fixed inset-0 bg-black/60 grid place-items-center z-30" onClick={onClose}>
      <div className="bg-slate-900 border border-slate-700 rounded-xl p-4 w-[640px] max-w-[95vw]" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center gap-3 mb-3">
          <h3 className="font-semibold">Propose a trade</h3>
          <span className="flex-1" />
          <label className="text-sm flex items-center gap-2">
            With
            <select className="bg-slate-800 rounded px-2 py-1" value={toId} onChange={(e) => { setToId(e.target.value); setGetProps([]); }}>
              {others.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </label>
        </div>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <div className="text-xs uppercase text-slate-400 mb-1">You give</div>
            <List items={mine} chosen={giveProps} onToggle={(i) => toggle(giveProps, setGiveProps, i)} />
            <div className="flex items-center gap-2 mt-2 text-sm">
              <span>Cash</span>
              <input type="number" min={0} max={meP.cash} className="bg-slate-800 rounded px-2 py-1 w-24" value={giveCash} onChange={(e) => setGiveCash(Math.max(0, Math.min(meP.cash, +e.target.value)))} />
              {meP.jailCards.length > 0 && (
                <>
                  <span className="ml-2">Jail cards</span>
                  <input type="number" min={0} max={meP.jailCards.length} className="bg-slate-800 rounded px-2 py-1 w-16" value={giveCards} onChange={(e) => setGiveCards(+e.target.value)} />
                </>
              )}
            </div>
          </div>
          <div>
            <div className="text-xs uppercase text-slate-400 mb-1">You get</div>
            <List items={theirs} chosen={getProps} onToggle={(i) => toggle(getProps, setGetProps, i)} />
            <div className="flex items-center gap-2 mt-2 text-sm">
              <span>Cash</span>
              <input type="number" min={0} max={themP?.cash ?? 0} className="bg-slate-800 rounded px-2 py-1 w-24" value={getCash} onChange={(e) => setGetCash(Math.max(0, +e.target.value))} />
              {(themP?.jailCards.length ?? 0) > 0 && (
                <>
                  <span className="ml-2">Jail cards</span>
                  <input type="number" min={0} max={themP?.jailCards.length} className="bg-slate-800 rounded px-2 py-1 w-16" value={getCards} onChange={(e) => setGetCards(+e.target.value)} />
                </>
              )}
            </div>
          </div>
        </div>
        <p className="text-xs text-slate-500 mt-3">Properties with buildings in their colour group can't be traded. Receiving a mortgaged property costs 10% interest immediately.</p>
        <div className="flex justify-end gap-2 mt-3">
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
          <Button onClick={propose} disabled={!toId || (giveCash === 0 && getCash === 0 && giveProps.length === 0 && getProps.length === 0 && giveCards === 0 && getCards === 0)}>
            Send offer
          </Button>
        </div>
      </div>
    </div>
  );
}

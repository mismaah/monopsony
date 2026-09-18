import { useEffect, useState } from "react";
import { useGame, send } from "@/store/game";
import { useAuth } from "@/store/auth";
import { Button } from "@/lib/ui";

function Countdown({ deadline }: { deadline: number }) {
  const [left, setLeft] = useState(0);
  useEffect(() => {
    const tick = () => setLeft(Math.max(0, Math.ceil((deadline - Date.now()) / 1000)));
    tick();
    const t = setInterval(tick, 250);
    return () => clearInterval(t);
  }, [deadline]);
  if (!deadline) return null;
  return <span className={`text-xs tabular-nums ${left <= 10 ? "text-rose-400" : "text-slate-400"}`}>{left}s</span>;
}

/**
 * ActionBar renders whatever the server says is legal for this player right
 * now, plus a one-line description of what the table is waiting for.
 */
export function ActionBar({ onTrade }: { onTrade: () => void }) {
  const { state, config, legal, waitingOn, deadline, animating, error } = useGame();
  const me = useAuth((s) => s.user);
  const [bid, setBid] = useState(0);
  if (!state || !config || !me) return null;

  const has = (t: string) => legal.some((a) => a.type === t);
  const act = (t: string) => legal.find((a) => a.type === t);
  const current = state.players[state.turn.playerIdx];
  const myTurn = current?.id === me.id;
  const phase = state.turn.phase;
  const waitingNames = waitingOn.map((id) => state.players.find((p) => p.id === id)?.name ?? id);
  const iAmWaited = waitingOn.includes(me.id);
  const busy = animating; // let the animation finish before offering the next action

  let headline: string;
  switch (phase) {
    case "pre_roll":
      headline = myTurn ? (current.inJail ? "You're in jail" : "Your turn — roll the dice") : `${current.name} is rolling`;
      break;
    case "resolving":
      headline = myTurn ? `Buy ${config.spaces[current.position].name} for $${config.spaces[current.position].price}?` : `${current.name} is deciding`;
      break;
    case "auction": {
      const a = state.auction!;
      const bidder = state.players.find((p) => p.id === a.turnId)?.name;
      headline = `Auction: ${config.spaces[a.spaceIndex].name} — high bid $${a.highBid}${a.highBidderId ? ` (${state.players.find((p) => p.id === a.highBidderId)?.name})` : ""} · ${bidder}'s bid`;
      break;
    }
    case "post_roll":
      headline = myTurn ? (state.turn.canRollAgain ? "Doubles! Roll again when ready" : "Build, trade, or end your turn") : `${current.name} is finishing their turn`;
      break;
    case "raising_funds": {
      const d = state.debts?.[0];
      const owed = d ? `$${d.amount}` : "";
      headline = iAmWaited ? `You owe ${owed} — sell, mortgage, or trade to raise it` : `${waitingNames.join(", ")} must raise funds`;
      break;
    }
    case "game_over":
      headline = `${state.players.find((p) => p.id === state.winnerId)?.name} wins!`;
      break;
    default:
      headline = "";
  }

  const trade = state.trade;
  const tradeForMe = trade && trade.toId === me.id;

  return (
    <div className="bg-slate-900/90 border border-slate-800 rounded-xl px-4 py-3 min-w-[420px] max-w-[680px]">
      <div className="flex items-center gap-3 mb-2">
        <span className="text-sm font-medium">{headline}</span>
        <span className="flex-1" />
        <Countdown deadline={deadline} />
      </div>
      {error && <div className="text-rose-400 text-xs mb-2">{error}</div>}
      <div className="flex flex-wrap gap-2 items-center">
        {has("UseJailCard") && <Button variant="secondary" disabled={busy} onClick={() => send("UseJailCard")}>Use jail card</Button>}
        {has("PayJailFine") && <Button variant="secondary" disabled={busy} onClick={() => send("PayJailFine")}>Pay ${config.rules.jailFine} fine</Button>}
        {has("RollDice") && <Button disabled={busy} onClick={() => send("RollDice")}>Roll dice</Button>}
        {has("BuyProperty") && <Button disabled={busy} onClick={() => send("BuyProperty")}>Buy</Button>}
        {has("DeclineBuy") && <Button variant="secondary" disabled={busy} onClick={() => send("DeclineBuy")}>{config.rules.auctionsEnabled ? "Auction it" : "Pass"}</Button>}
        {has("PlaceBid") && (
          <div className="flex items-center gap-1">
            <input
              type="number"
              className="w-24 bg-slate-800 rounded px-2 py-1 text-sm"
              min={act("PlaceBid")!.minBid}
              value={Math.max(bid, act("PlaceBid")!.minBid ?? 1)}
              onChange={(e) => setBid(+e.target.value)}
            />
            <Button disabled={busy} onClick={() => send("PlaceBid", { amount: Math.max(bid, act("PlaceBid")!.minBid ?? 1) })}>Bid</Button>
            <Button variant="secondary" disabled={busy} onClick={() => send("PlaceBid", { amount: (state.auction?.highBid ?? 0) + Math.max(10, Math.round((config.spaces[state.auction!.spaceIndex].price ?? 0) / 10)) })}>
              +{Math.max(10, Math.round((config.spaces[state.auction!.spaceIndex].price ?? 0) / 10))}
            </Button>
          </div>
        )}
        {has("PassBid") && <Button variant="secondary" disabled={busy} onClick={() => send("PassBid")}>Pass</Button>}
        {has("EndTurn") && <Button disabled={busy} onClick={() => send("EndTurn")}>{state.turn.canRollAgain ? "Continue" : "End turn"}</Button>}
        {has("ProposeTrade") && <Button variant="secondary" disabled={busy} onClick={onTrade}>Trade…</Button>}
        {has("DeclareBankruptcy") && <Button variant="danger" disabled={busy} onClick={() => send("DeclareBankruptcy")}>Declare bankruptcy</Button>}
        {tradeForMe && (
          <>
            <Button disabled={busy} onClick={() => send("AcceptTrade", { tradeId: trade.id })}>Accept trade</Button>
            <Button variant="danger" disabled={busy} onClick={() => send("RejectTrade", { tradeId: trade.id })}>Reject</Button>
          </>
        )}
        {trade && trade.fromId === me.id && (
          <Button variant="ghost" onClick={() => send("RejectTrade", { tradeId: trade.id })}>Cancel trade</Button>
        )}
        {legal.length === 0 && phase !== "game_over" && <span className="text-xs text-slate-500">Waiting for {waitingNames.join(", ")}…</span>}
      </div>
    </div>
  );
}

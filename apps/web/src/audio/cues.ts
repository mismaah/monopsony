import type { GameEvent } from "@monopsony/protocol";
import type { CueName, PlayOpts } from "./sfx";

export type Cue = { name: CueName } & PlayOpts;

// Cash movements already voiced by a more specific cue (passGo, buy, build…).
const VOICED_CASH = new Set(["salary", "purchase", "auction", "build", "sell_building"]);

/**
 * Which sound(s) an event makes when it reaches the head of the animation
 * queue. `me` is the viewer's player id: things that happen *to you* are
 * louder or distinct from the same thing happening to someone else.
 * Token steps are timed by the queue itself and are not listed here.
 */
export function cuesFor(ev: GameEvent, me: string | null): Cue[] {
  const mine = (id: string | undefined) => !!me && id === me;
  const others = (name: CueName): Cue => ({ name, gain: 0.5 });
  switch (ev.type) {
    case "GameStarted":
      return [{ name: "start" }];
    case "DiceRolled":
      return ev.payload.doubles ? [{ name: "dice" }, { name: "doubles", delay: 0.7 }] : [{ name: "dice" }];
    case "TokenMoved":
      return ev.payload.viaCard && !ev.payload.backward ? [{ name: "teleport" }] : [];
    case "SalaryCollected":
      return [{ name: "passGo" }];
    case "CashChanged": {
      if (VOICED_CASH.has(ev.payload.reason)) return [];
      const name: CueName = ev.payload.delta > 0 ? "cashIn" : "cashOut";
      return mine(ev.payload.playerId) ? [{ name }] : [others(name)];
    }
    case "PropertyBought":
      return [{ name: "buy" }];
    case "PurchaseDeclined":
      return [others("deny")];
    case "AuctionStarted":
      return [{ name: "bid" }, { name: "bid", delay: 0.12 }, { name: "bid", delay: 0.24 }];
    case "BidPlaced":
      return [{ name: "bid" }];
    case "AuctionEnded":
      return ev.payload.winnerId ? [{ name: "buy" }] : [others("deny")];
    case "CardDrawn":
      return [{ name: "card" }];
    case "SentToJail":
      return [{ name: "jail" }];
    case "LeftJail":
      return [{ name: "chime" }];
    case "HouseBuilt":
      return [{ name: "build" }];
    case "HouseSold":
      return [{ name: "sell" }];
    case "TradeProposed":
      return mine(ev.payload.trade.toId) ? [{ name: "trade" }] : [others("trade")];
    case "TradeAccepted":
      return [{ name: "chime" }];
    case "TradeRejected":
      return [others("deny")];
    case "DebtIncurred":
      return mine(ev.payload.debt.debtorId) ? [{ name: "alarm" }] : [others("alarm")];
    case "PlayerBankrupt":
      return [{ name: "bankrupt" }];
    case "TurnChanged":
      return mine(ev.payload.playerId) ? [{ name: "yourTurn" }] : [];
    case "GameEnded":
      return [{ name: mine(ev.payload.winnerId) ? "win" : "end" }];
    default:
      return [];
  }
}

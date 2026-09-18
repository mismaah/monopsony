import type { GameEvent } from "@monopsony/protocol";

interface Names {
  spaceName: (i: number) => string;
  playerName: (id: string) => string;
}

type Line = { text: string; kind: "info" | "money" | "alert" };

/** Human-readable log line for an event, or null for noise. */
export function describeEvent(ev: GameEvent, n: Names): Line | null {
  const p = ev.payload as unknown as Record<string, unknown>;
  const who = (k = "playerId") => n.playerName(p[k] as string);
  switch (ev.type) {
    case "GameStarted":
      return { text: "The game begins!", kind: "alert" };
    case "DiceRolled": {
      const d = ev.payload.dice;
      return { text: `${who()} rolls ${d[0]} + ${d[1]}${ev.payload.doubles ? " — doubles!" : ""}`, kind: "info" };
    }
    case "SalaryCollected":
      return { text: `${who()} passes Go and collects $${ev.payload.amount}`, kind: "money" };
    case "PurchaseOffered":
      return { text: `${who()} lands on ${n.spaceName(ev.payload.space)} ($${ev.payload.price})`, kind: "info" };
    case "PropertyBought":
      return { text: `${who()} buys ${n.spaceName(ev.payload.space)} for $${ev.payload.price}`, kind: "money" };
    case "PurchaseDeclined":
      return { text: `${who()} declines ${n.spaceName(ev.payload.space)}`, kind: "info" };
    case "AuctionStarted":
      return { text: `Auction for ${n.spaceName(ev.payload.space)} begins`, kind: "alert" };
    case "BidPlaced":
      return { text: `${who()} bids $${ev.payload.amount}`, kind: "info" };
    case "BidPassed":
      return { text: `${who()} passes`, kind: "info" };
    case "AuctionEnded":
      return ev.payload.winnerId
        ? {
            text: `${n.playerName(ev.payload.winnerId)} wins ${n.spaceName(ev.payload.space)} for $${ev.payload.amount}`,
            kind: "money",
          }
        : { text: `Nobody bid on ${n.spaceName(ev.payload.space)}`, kind: "info" };
    case "RentPaid":
      return {
        text: `${who("payerId")} pays $${ev.payload.amount} rent to ${who("ownerId")} for ${n.spaceName(ev.payload.space)}`,
        kind: "money",
      };
    case "TaxPaid":
      return { text: `${who()} pays $${ev.payload.amount} tax`, kind: "money" };
    case "CardDrawn":
      return { text: `${who()} draws: “${ev.payload.text}”`, kind: "info" };
    case "SentToJail":
      return { text: `${who()} goes to jail`, kind: "alert" };
    case "JailFinePaid":
      return { text: `${who()} pays the $${ev.payload.amount} fine`, kind: "money" };
    case "JailCardUsed":
      return { text: `${who()} uses a Get Out of Jail Free card`, kind: "info" };
    case "LeftJail":
      return ev.payload.how === "doubles" ? { text: `${who()} rolls doubles and leaves jail`, kind: "info" } : null;
    case "JailTurnServed":
      return { text: `${who()} stays in jail (${ev.payload.turns})`, kind: "info" };
    case "HouseBuilt":
      return {
        text: `${who()} builds ${ev.payload.houses === 5 ? "a hotel" : "a house"} on ${n.spaceName(ev.payload.space)}`,
        kind: "money",
      };
    case "HouseSold":
      return { text: `${who()} sells a building on ${n.spaceName(ev.payload.space)} for $${ev.payload.refund}`, kind: "money" };
    case "Mortgaged":
      return { text: `${who()} mortgages ${n.spaceName(ev.payload.space)} for $${ev.payload.amount}`, kind: "money" };
    case "Unmortgaged":
      return { text: `${who()} lifts the mortgage on ${n.spaceName(ev.payload.space)}`, kind: "money" };
    case "TradeProposed":
      return {
        text: `${n.playerName(ev.payload.trade.fromId)} proposes a trade to ${n.playerName(ev.payload.trade.toId)}`,
        kind: "alert",
      };
    case "TradeAccepted":
      return { text: "Trade accepted", kind: "money" };
    case "TradeRejected":
      return { text: ev.payload.byId ? "Trade rejected" : "Trade cancelled", kind: "info" };
    case "DebtIncurred":
      return {
        text: `${n.playerName(ev.payload.debt.debtorId)} owes $${ev.payload.debt.amount} and must raise funds`,
        kind: "alert",
      };
    case "DebtSettled":
      return { text: `${n.playerName(ev.payload.debt.debtorId)} settles $${ev.payload.debt.amount}`, kind: "money" };
    case "PlayerBankrupt":
      return { text: `${who()} is bankrupt!`, kind: "alert" };
    case "FreeParkingCollected":
      return { text: `${who()} collects $${ev.payload.amount} from Free Parking`, kind: "money" };
    case "TurnChanged":
      return { text: `${who()}'s turn`, kind: "info" };
    case "GameEnded":
      return { text: `${n.playerName(ev.payload.winnerId)} wins the game!`, kind: "alert" };
    default:
      return null;
  }
}

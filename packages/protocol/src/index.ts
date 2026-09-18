// Shared wire types. game.ts and protocol.ts are generated from Go (task gen);
// this file adds the hand-written pieces: command payloads and event unions.
export * from "./protocol";
export type {
  SpaceType, SpaceDef, CardEffect, CardDef, Rules, Config, Phase, Player, SpaceState, Turn, Auction, TradeSide, Trade, Debt, Decks, Bank, State, Action,
  EventEnvelope, GameStarted, DiceRolled, TokenMoved, CashChanged, SalaryCollected, PurchaseOffered, PropertyBought, PurchaseDeclined, AuctionStarted,
  BidPlaced, BidPassed, AuctionEnded, RentPaid, TaxPaid, CardDrawn, CardKept, UtilityRoll, SentToJail, JailFinePaid, JailCardUsed, LeftJail, JailTurnServed,
  HouseBuilt, HouseSold, Mortgaged, Unmortgaged, TradeProposed, TradeAccepted, TradeRejected, PropertyTransferred, DebtIncurred, DebtSettled, PlayerBankrupt,
  FreeParkingCollected, PhaseChanged, TurnChanged, GameEnded,
} from "./game";

import type * as G from "./game";

/**
 * Payloads the client sends inside protocol.Command. The server injects
 * playerId from the session, so it is never part of the payload.
 */
export interface CommandPayloads {
  RollDice: Record<string, never>;
  BuyProperty: Record<string, never>;
  DeclineBuy: Record<string, never>;
  PlaceBid: { amount: number };
  PassBid: Record<string, never>;
  EndTurn: Record<string, never>;
  BuildHouse: { space: number };
  SellHouse: { space: number };
  Mortgage: { space: number };
  Unmortgage: { space: number };
  ProposeTrade: { toId: string; give: G.TradeSide; receive: G.TradeSide };
  AcceptTrade: { tradeId: string };
  RejectTrade: { tradeId: string };
  PayJailFine: Record<string, never>;
  UseJailCard: Record<string, never>;
  DeclareBankruptcy: Record<string, never>;
  Surrender: Record<string, never>;
}
export type CommandType = keyof CommandPayloads;

/** Event payloads keyed by their wire type name. */
export interface EventPayloads {
  GameStarted: G.GameStarted;
  DiceRolled: G.DiceRolled;
  TokenMoved: G.TokenMoved;
  CashChanged: G.CashChanged;
  SalaryCollected: G.SalaryCollected;
  PurchaseOffered: G.PurchaseOffered;
  PropertyBought: G.PropertyBought;
  PurchaseDeclined: G.PurchaseDeclined;
  AuctionStarted: G.AuctionStarted;
  BidPlaced: G.BidPlaced;
  BidPassed: G.BidPassed;
  AuctionEnded: G.AuctionEnded;
  RentPaid: G.RentPaid;
  TaxPaid: G.TaxPaid;
  CardDrawn: G.CardDrawn;
  CardKept: G.CardKept;
  UtilityRoll: G.UtilityRoll;
  SentToJail: G.SentToJail;
  JailFinePaid: G.JailFinePaid;
  JailCardUsed: G.JailCardUsed;
  LeftJail: G.LeftJail;
  JailTurnServed: G.JailTurnServed;
  HouseBuilt: G.HouseBuilt;
  HouseSold: G.HouseSold;
  Mortgaged: G.Mortgaged;
  Unmortgaged: G.Unmortgaged;
  TradeProposed: G.TradeProposed;
  TradeAccepted: G.TradeAccepted;
  TradeRejected: G.TradeRejected;
  PropertyTransferred: G.PropertyTransferred;
  DebtIncurred: G.DebtIncurred;
  DebtSettled: G.DebtSettled;
  PlayerBankrupt: G.PlayerBankrupt;
  FreeParkingCollected: G.FreeParkingCollected;
  PhaseChanged: G.PhaseChanged;
  TurnChanged: G.TurnChanged;
  GameEnded: G.GameEnded;
}
export type EventType = keyof EventPayloads;

/** A typed game event as delivered over the socket. */
export type GameEvent = {
  [K in EventType]: { seq: number; type: K; payload: EventPayloads[K] };
}[EventType];

/** Phases as string constants (the Go type is a string alias). */
export const Phases = {
  PreRoll: "pre_roll",
  Resolving: "resolving",
  Auction: "auction",
  PostRoll: "post_roll",
  RaisingFunds: "raising_funds",
  GameOver: "game_over",
} as const;

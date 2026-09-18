import { describe, expect, it } from "vitest";
import type { GameEvent } from "@monopsony/protocol";
import { cuesFor } from "./cues";

const ev = <K extends GameEvent["type"]>(type: K, payload: Extract<GameEvent, { type: K }>["payload"]) =>
  ({ seq: 1, type, payload }) as GameEvent;

describe("cuesFor", () => {
  it("rolls dice, and chimes on doubles", () => {
    expect(cuesFor(ev("DiceRolled", { playerId: "a", dice: [2, 5], doubles: false, doublesCount: 0, inJail: false }), "a").map((c) => c.name)).toEqual(["dice"]);
    expect(cuesFor(ev("DiceRolled", { playerId: "a", dice: [4, 4], doubles: true, doublesCount: 1, inJail: false }), "a").map((c) => c.name)).toEqual(["dice", "doubles"]);
  });

  it("only announces your own turn", () => {
    expect(cuesFor(ev("TurnChanged", { playerId: "a", number: 3 }), "a")).toEqual([{ name: "yourTurn" }]);
    expect(cuesFor(ev("TurnChanged", { playerId: "a", number: 3 }), "b")).toEqual([]);
    expect(cuesFor(ev("TurnChanged", { playerId: "a", number: 3 }), null)).toEqual([]);
  });

  it("skips cash movements that a dedicated cue already voices", () => {
    expect(cuesFor(ev("CashChanged", { playerId: "a", delta: 200, balance: 1700, reason: "salary" }), "a")).toEqual([]);
    expect(cuesFor(ev("CashChanged", { playerId: "a", delta: -50, balance: 1450, reason: "rent" }), "a")).toEqual([{ name: "cashOut" }]);
  });

  it("plays other players' money quieter", () => {
    const [c] = cuesFor(ev("CashChanged", { playerId: "a", delta: 50, balance: 1550, reason: "rent" }), "b");
    expect(c.name).toBe("cashIn");
    expect(c.gain).toBeLessThan(1);
  });

  it("distinguishes winning from losing", () => {
    expect(cuesFor(ev("GameEnded", { winnerId: "a", reason: "last_standing" }), "a")).toEqual([{ name: "win" }]);
    expect(cuesFor(ev("GameEnded", { winnerId: "a", reason: "last_standing" }), "b")).toEqual([{ name: "end" }]);
  });

  it("teleports only on forward card moves", () => {
    const base = { playerId: "a", from: 7, to: 24, passedGo: false, viaCard: true, backward: false };
    expect(cuesFor(ev("TokenMoved", base), "a")).toEqual([{ name: "teleport" }]);
    expect(cuesFor(ev("TokenMoved", { ...base, backward: true }), "a")).toEqual([]);
    expect(cuesFor(ev("TokenMoved", { ...base, viaCard: false }), "a")).toEqual([]);
  });
});

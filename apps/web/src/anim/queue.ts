import type { GameEvent } from "@monopsony/protocol";
import { T } from "./timing";
import { describeEvent } from "./describe";

/** The slice of the game store the queue reads and writes. */
export interface QueueBinding {
  get: () => {
    positions: Record<string, number>;
    cash: Record<string, number>;
    dice: { faces: [number, number]; rollId: number };
    log: { seq: number; text: string; kind: "info" | "money" | "alert" }[];
    config: { spaces: { name: string }[] } | null;
    state: { players: { id: string; name: string }[] } | null;
    seats: { playerId: string; name: string }[];
  };
  set: (partial: Record<string, unknown>) => void;
}

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));
const BOARD = 40;

/**
 * AnimationQueue plays events one at a time, nudging the "view" fields of the
 * store (positions, cash, dice, card) so the scene animates in order while the
 * authoritative state has already jumped ahead.
 */
export class AnimationQueue {
  private items: GameEvent[] = [];
  private running = false;
  private gen = 0;
  private b!: QueueBinding;

  bind(b: QueueBinding) {
    this.b = b;
  }

  clear() {
    this.items = [];
    this.gen++;
    this.running = false;
    this.b?.set({ animating: false, card: null, highlight: null });
  }

  push(ev: GameEvent) {
    this.items.push(ev);
    if (!this.running) void this.run();
  }

  private async run() {
    this.running = true;
    const gen = this.gen;
    this.b.set({ animating: true });
    while (this.items.length && gen === this.gen) {
      const ev = this.items.shift()!;
      this.logLine(ev);
      try {
        await this.play(ev, gen);
      } catch (e) {
        console.warn("animation failed", ev, e);
      }
    }
    if (gen === this.gen) {
      this.running = false;
      this.b.set({ animating: false });
    }
  }

  private logLine(ev: GameEvent) {
    const s = this.b.get();
    const line = describeEvent(ev, {
      spaceName: (i) => s.config?.spaces[i]?.name ?? `#${i}`,
      playerName: (id) =>
        s.state?.players.find((p) => p.id === id)?.name ?? s.seats.find((x) => x.playerId === id)?.name ?? id,
    });
    if (!line) return;
    this.b.set({ log: [...s.log.slice(-149), { seq: ev.seq, ...line }] });
  }

  private async play(ev: GameEvent, gen: number) {
    const s = this.b.get();
    switch (ev.type) {
      case "DiceRolled":
        this.b.set({ dice: { faces: ev.payload.dice as [number, number], rollId: s.dice.rollId + 1 } });
        await sleep(T.diceRoll);
        break;
      case "TokenMoved": {
        const { playerId, from, to, viaCard, backward } = ev.payload;
        if (viaCard && !backward) {
          this.b.set({ positions: { ...this.b.get().positions, [playerId]: to } });
          await sleep(T.tokenTeleport);
          break;
        }
        let pos = from;
        const dir = backward ? -1 : 1;
        while (pos !== to && gen === this.gen) {
          pos = (pos + dir + BOARD) % BOARD;
          this.b.set({ positions: { ...this.b.get().positions, [playerId]: pos } });
          await sleep(T.tokenStep);
        }
        break;
      }
      case "CashChanged":
        this.b.set({ cash: { ...this.b.get().cash, [ev.payload.playerId]: ev.payload.balance } });
        break;
      case "CardDrawn":
        this.b.set({ card: { deck: ev.payload.deck, text: ev.payload.text, playerId: ev.payload.playerId } });
        await sleep(T.cardShow);
        this.b.set({ card: null });
        break;
      case "PropertyBought":
      case "AuctionEnded":
      case "PropertyTransferred":
        this.b.set({ highlight: ev.payload.space });
        await sleep(T.highlight);
        this.b.set({ highlight: null });
        break;
      case "HouseBuilt":
      case "HouseSold":
        await sleep(T.buildingPop);
        break;
      case "SentToJail":
        await sleep(300);
        break;
      default:
        break;
    }
  }
}

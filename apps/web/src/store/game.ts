import { create } from "zustand";
import type { Action, Config, GameEvent, Lobby, SeatInfo, Snapshot, State, Update } from "@monopsony/protocol";
import { socket } from "@/api/ws";
import { AnimationQueue, type CashPop } from "@/anim/queue";
import { useAuth } from "@/store/auth";
import { sfx } from "@/audio/sfx";

export interface LogLine {
  seq: number;
  text: string;
  kind: "info" | "money" | "alert";
}

/** Dice as last rolled, with a counter so the scene can detect a new roll. */
export interface DiceView {
  faces: [number, number];
  rollId: number;
}

export interface CardView {
  deck: string;
  text: string;
  playerId: string;
}

interface GameStore {
  gameId: string | null;
  lobby: Lobby | null;
  config: Config | null;
  /** Authoritative state from the server (jumps ahead of the animation). */
  state: State | null;
  seats: SeatInfo[];
  legal: Action[];
  waitingOn: string[];
  deadline: number;
  /** What the 3D scene shows: updated step by step by the animation queue. */
  positions: Record<string, number>;
  cash: Record<string, number>;
  /** Recent cash movements, shown as floating amounts on the players rail. */
  cashPops: CashPop[];
  dice: DiceView;
  card: CardView | null;
  highlight: number | null; // space flashed by a purchase/auction
  selectedSpace: number | null;
  log: LogLine[];
  chat: { name: string; text: string; at: number }[];
  animating: boolean;
  error: string | null;
  /** Set when the host removed us from the open game; a bot has our seat. */
  kicked: boolean;

  open: (gameId: string) => void;
  close: () => void;
  select: (space: number | null) => void;
  setError: (e: string | null) => void;
}

const queue = new AnimationQueue();

// Highest event seq handed to the queue. Events are strictly sequential, so
// anything at or below it is a duplicate delivery and must not animate/log twice.
let lastEventSeq = 0;

export const useGame = create<GameStore>((set, get) => {
  // Wire socket handlers once. Snapshots reset the view; events animate it;
  // Updates carry the authoritative state + legal actions.
  socket.on("Lobby", (p) => {
    const l = p as Lobby;
    if (l.gameId !== get().gameId) return;
    set({ lobby: l, seats: l.seats });
  });
  socket.on("Snapshot", (p) => {
    const s = p as Snapshot;
    if (s.gameId !== get().gameId || !s.state || !s.config) return;
    queue.clear();
    lastEventSeq = s.state.seq;
    const positions: Record<string, number> = {};
    const cash: Record<string, number> = {};
    for (const pl of s.state.players) {
      positions[pl.id] = pl.position;
      cash[pl.id] = pl.cash;
    }
    set({
      config: s.config,
      state: s.state,
      seats: s.seats,
      legal: s.legal ?? [],
      waitingOn: s.waitingOn ?? [],
      deadline: s.deadline ?? 0,
      positions,
      cash,
      dice: { faces: [s.state.turn.lastRoll[0] || 1, s.state.turn.lastRoll[1] || 1], rollId: 0 },
      card: null,
      animating: false,
    });
  });
  socket.on("Event", (p) => {
    const ev = p as GameEvent & { gameId: string };
    if (ev.gameId !== get().gameId || ev.seq <= lastEventSeq) return;
    lastEventSeq = ev.seq;
    queue.push(ev);
  });
  socket.on("Update", (p) => {
    const u = p as Update;
    if (u.gameId !== get().gameId || !u.state) return;
    set({ state: u.state, seats: u.seats, legal: u.actions ?? [], waitingOn: u.waitingOn ?? [], deadline: u.deadline ?? 0 });
  });
  socket.on("Kicked", (p) => {
    const k = p as { gameId: string };
    if (k.gameId !== get().gameId) return;
    set({ kicked: true, legal: [] });
  });
  socket.on("Chat", (p) => {
    const c = p as { gameId: string; name: string; text: string; at: number };
    if (c.gameId !== get().gameId) return;
    set({ chat: [...get().chat.slice(-99), { name: c.name, text: c.text, at: c.at }] });
  });

  queue.bind({
    get: () => ({ ...get(), me: useAuth.getState().user?.id ?? null }),
    set: (partial) => set(partial),
  });

  return {
    gameId: null,
    lobby: null,
    config: null,
    state: null,
    seats: [],
    legal: [],
    waitingOn: [],
    deadline: 0,
    positions: {},
    cash: {},
    cashPops: [],
    dice: { faces: [1, 1], rollId: 0 },
    card: null,
    highlight: null,
    selectedSpace: null,
    log: [],
    chat: [],
    animating: false,
    error: null,
    kicked: false,

    open(gameId) {
      if (get().gameId === gameId) return;
      const prev = get().gameId;
      if (prev) socket.leave(prev);
      queue.clear();
      lastEventSeq = 0;
      set({ gameId, lobby: null, config: null, state: null, seats: [], legal: [], log: [], chat: [], card: null, selectedSpace: null, kicked: false });
      socket.join(gameId);
    },
    close() {
      const prev = get().gameId;
      if (prev) socket.leave(prev);
      queue.clear();
      set({ gameId: null, lobby: null, state: null, config: null });
    },
    select(space) {
      set({ selectedSpace: space });
    },
    setError(e) {
      if (e) sfx.play("error");
      set({ error: e });
    },
  };
});

/** Send a command for the open game; errors surface in the store. */
export async function send<K extends Parameters<typeof socket.command>[1]>(type: K, payload?: Parameters<typeof socket.command<K>>[2]) {
  const gameId = useGame.getState().gameId;
  if (!gameId) return;
  try {
    await socket.command(gameId, type, payload);
    useGame.getState().setError(null);
  } catch (e) {
    useGame.getState().setError((e as Error).message);
  }
}

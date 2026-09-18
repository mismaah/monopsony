import { useEffect, useRef, useState, type ReactNode } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { useNavigate } from "react-router-dom";
import { useGame } from "@/store/game";
import { useAuth } from "@/store/auth";
import { socket } from "@/api/ws";
import { Button, seatColors } from "@/lib/ui";
import type { Config, TradeSide } from "@monopsony/protocol";

/** Boards name their own decks via the spaces that draw from them ("Tide", "Harbour Fund", …). */
export function deckLabel(config: Config | null, deck: string | undefined): string {
  const type = deck === "chance" ? "chance" : "community_chest";
  return config?.spaces.find((s) => s.type === type)?.name ?? (deck === "chance" ? "Chance" : "Community");
}

/** Card flip for either deck. The deck's display name is whatever the board calls its spaces. */
export function CardOverlay() {
  const card = useGame((s) => s.card);
  const label = useGame((s) => deckLabel(s.config, card?.deck));
  return (
    <AnimatePresence>
      {card && (
        <motion.div
          key={card.text}
          initial={{ opacity: 0, rotateY: 90, scale: 0.8 }}
          animate={{ opacity: 1, rotateY: 0, scale: 1 }}
          exit={{ opacity: 0, y: -40 }}
          transition={{ duration: 0.45, ease: "easeOut" }}
          className={`absolute left-1/2 top-1/3 -translate-x-1/2 w-72 rounded-xl p-5 shadow-2xl border ${card.deck === "chance" ? "bg-sky-100 border-sky-300 text-sky-950" : "bg-amber-100 border-amber-300 text-amber-950"}`}
        >
          <div className="text-xs uppercase tracking-widest opacity-70 mb-2">{label}</div>
          <div className="font-medium leading-snug">{card.text}</div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

/** Pending trade panel: who offers what, with a mini deed card for every property on the table. */
export function TradeBanner() {
  const { state, config, select } = useGame();
  const me = useAuth((s) => s.user);
  const t = state?.trade;
  if (!t || !state || !config) return null;
  const forMe = t.toId === me?.id;
  const name = (id: string) => state.players.find((p) => p.id === id)?.name ?? id;
  const seat = (id: string) => seatColors[Math.max(0, state.players.findIndex((p) => p.id === id)) % seatColors.length];
  const interestPct = config.rules.mortgageInterestPct;

  /** Compact deed: colour band, price, headline rent, and anything the recipient should know before accepting. */
  const DeedCard = ({ space, fromId, toId }: { space: number; fromId: string; toId: string }) => {
    const def = config.spaces[space];
    const st = state.spaces[space];
    const rent = def.rent ?? [];
    const price = def.price ?? 0;
    const mortgage = Math.floor(price / 2);
    const interest = Math.floor((mortgage * interestPct) / 100);
    // Colour-group maths after the swap: does this hand the recipient a full set, or break the giver's?
    const group = def.group;
    const inGroup = group ? config.spaces.map((_, j) => j).filter((j) => config.spaces[j].group === group) : [];
    const ownerAfter = (j: number) => (t.give.properties.includes(j) ? t.toId : t.receive.properties.includes(j) ? t.fromId : state.spaces[j].ownerId);
    const recvAfter = inGroup.filter((j) => ownerAfter(j) === toId).length;
    const giverBefore = inGroup.filter((j) => state.spaces[j].ownerId === fromId).length;
    const completesSet = inGroup.length > 0 && recvAfter === inGroup.length;
    const breaksSet = inGroup.length > 0 && giverBefore === inGroup.length;
    const typeLabel = def.type === "railroad" ? "Railroad" : def.type === "utility" ? "Utility" : `${recvAfter}/${inGroup.length} of set`;
    const rentLine =
      def.type === "street"
        ? `Rent $${rent[0] ?? 0} · set $${(rent[0] ?? 0) * 2} · hotel $${rent[5] ?? 0}`
        : def.type === "railroad"
          ? `Rent $${rent[0] ?? 0} – $${rent[3] ?? 0}`
          : `Rent ${rent[0] ?? 0}× – ${rent[1] ?? 0}× dice`;
    return (
      <button
        type="button"
        onClick={() => select(space)}
        title="Show full deed"
        className="w-40 text-left bg-slate-800/80 hover:bg-slate-800 border border-slate-700 rounded-lg overflow-hidden transition-colors"
      >
        <div className="px-2 py-1 text-xs font-semibold truncate" style={{ background: def.color || "#475569", color: "#fff" }}>
          {def.name}
        </div>
        <div className="px-2 py-1.5 text-[11px] leading-tight space-y-0.5">
          <div className="flex justify-between text-slate-300">
            <span>{typeLabel}</span>
            <span>${price}</span>
          </div>
          <div className="text-slate-400">{rentLine}</div>
          {def.type === "street" && <div className="text-slate-500">House ${def.houseCost ?? 0}</div>}
          {st.mortgaged && (
            <div className="text-amber-300">
              Mortgaged · ${interest} interest due, ${mortgage + interest} to lift
            </div>
          )}
          {completesSet && <div className="text-emerald-300">Completes {name(toId)}'s set</div>}
          {breaksSet && <div className="text-rose-300">Breaks {name(fromId)}'s set</div>}
        </div>
      </button>
    );
  };

  const Side = ({ side, fromId, toId, label, tone }: { side: TradeSide; fromId: string; toId: string; label: string; tone: string }) => {
    const empty = !side.cash && !side.properties.length && !side.jailCards;
    return (
      <div className="flex-1 min-w-0">
        <div className={`text-[10px] uppercase tracking-wider mb-1.5 ${tone}`}>{label}</div>
        {empty && <div className="text-xs text-slate-500 italic">nothing</div>}
        <div className="flex flex-wrap gap-1.5">
          {side.cash > 0 && <Chip>💵 ${side.cash}</Chip>}
          {side.jailCards > 0 && <Chip>🃏 {side.jailCards} Get Out of Jail Free</Chip>}
          {side.properties.map((i) => (
            <DeedCard key={i} space={i} fromId={fromId} toId={toId} />
          ))}
        </div>
      </div>
    );
  };

  return (
    <div className={`bg-slate-900/95 border rounded-xl px-4 py-3 text-sm w-[640px] max-w-[95vw] shadow-xl ${forMe ? "border-emerald-400" : "border-slate-700"}`}>
      <div className="flex items-center gap-2 mb-2">
        <span className="w-2 h-2 rounded-full" style={{ background: seat(t.fromId) }} />
        <span className="font-medium">{name(t.fromId)}</span>
        <span className="text-slate-400">proposes a trade with</span>
        <span className="w-2 h-2 rounded-full" style={{ background: seat(t.toId) }} />
        <span className="font-medium">{name(t.toId)}</span>
        <span className="flex-1" />
        <span className="text-xs text-slate-500">Click a property for its full deed</span>
      </div>
      <div className="flex gap-4">
        <Side side={t.give} fromId={t.fromId} toId={t.toId} label={`${name(t.fromId)} gives`} tone="text-emerald-300" />
        <div className="self-center text-slate-500 text-lg">⇄</div>
        <Side side={t.receive} fromId={t.toId} toId={t.fromId} label={`${name(t.toId)} gives`} tone="text-amber-300" />
      </div>
    </div>
  );
}

function Chip({ children }: { children: ReactNode }) {
  return <span className="inline-flex items-center h-7 px-2 rounded-md bg-slate-800/80 border border-slate-700 text-xs whitespace-nowrap">{children}</span>;
}

export function GameOverOverlay() {
  const { state } = useGame();
  const nav = useNavigate();
  if (!state || state.turn.phase !== "game_over") return null;
  const winner = state.players.find((p) => p.id === state.winnerId);
  return (
    <div className="absolute inset-0 grid place-items-center bg-black/50 z-20">
      <motion.div initial={{ scale: 0.8, opacity: 0 }} animate={{ scale: 1, opacity: 1 }} className="bg-slate-900 border border-amber-400/50 rounded-2xl p-8 text-center">
        <div className="text-5xl mb-2">🏆</div>
        <h2 className="text-2xl font-bold">{winner?.name} wins!</h2>
        <p className="text-slate-400 mt-1">After {state.turn.number} turns</p>
        <Button className="mt-4" onClick={() => nav("/lobby")}>
          Back to lobby
        </Button>
      </motion.div>
    </div>
  );
}

/** Shown to a player the host removed: their seat plays on under a bot. */
export function KickedOverlay() {
  const kicked = useGame((s) => s.kicked);
  const nav = useNavigate();
  if (!kicked) return null;
  return (
    <div className="absolute inset-0 grid place-items-center bg-black/50 z-20">
      <motion.div initial={{ scale: 0.8, opacity: 0 }} animate={{ scale: 1, opacity: 1 }} className="bg-slate-900 border border-rose-400/50 rounded-2xl p-8 text-center max-w-sm">
        <h2 className="text-2xl font-bold">You were removed</h2>
        <p className="text-slate-400 mt-1">The host took you out of this game. A bot is playing your seat from here on.</p>
        <Button className="mt-4" onClick={() => nav("/lobby")}>
          Back to lobby
        </Button>
      </motion.div>
    </div>
  );
}

/** Event log + chat in one scrolling panel. */
export function LogPanel() {
  const { log, chat, gameId } = useGame();
  const [text, setText] = useState("");
  const ref = useRef<HTMLDivElement>(null);
  const lines = [
    ...log.map((l) => ({ key: `e${l.seq}`, at: l.seq, text: l.text, cls: l.kind === "alert" ? "text-amber-300" : l.kind === "money" ? "text-emerald-300" : "text-slate-300" })),
    ...chat.map((c, i) => ({ key: `c${i}`, at: Number.MAX_SAFE_INTEGER - chat.length + i, text: `${c.name}: ${c.text}`, cls: "text-sky-300" })),
  ];
  useEffect(() => {
    ref.current?.scrollTo({ top: ref.current.scrollHeight });
  }, [log.length, chat.length]);
  return (
    <div className="w-80 h-56 bg-slate-900/85 border border-slate-800 rounded-xl flex flex-col">
      <div ref={ref} className="flex-1 overflow-auto px-3 py-2 text-xs space-y-0.5">
        {lines.slice(-80).map((l) => (
          <div key={l.key} className={l.cls}>
            {l.text}
          </div>
        ))}
      </div>
      <form
        className="border-t border-slate-800 flex"
        onSubmit={(e) => {
          e.preventDefault();
          if (gameId && text.trim()) socket.chat(gameId, text.trim());
          setText("");
        }}
      >
        <input className="flex-1 bg-transparent px-3 py-1.5 text-xs outline-none" placeholder="Say something…" value={text} onChange={(e) => setText(e.target.value)} maxLength={200} />
      </form>
    </div>
  );
}

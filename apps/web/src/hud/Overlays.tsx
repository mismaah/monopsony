import { useEffect, useRef, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { useNavigate } from "react-router-dom";
import { useGame } from "@/store/game";
import { useAuth } from "@/store/auth";
import { socket } from "@/api/ws";
import { Button } from "@/lib/ui";

/** Chance / Community Chest card flip. */
export function CardOverlay() {
  const card = useGame((s) => s.card);
  return (
    <AnimatePresence>
      {card && (
        <motion.div
          key={card.text}
          initial={{ opacity: 0, rotateY: 90, scale: 0.8 }}
          animate={{ opacity: 1, rotateY: 0, scale: 1 }}
          exit={{ opacity: 0, y: -40 }}
          transition={{ duration: 0.45, ease: "easeOut" }}
          className={`absolute left-1/2 top-1/3 -translate-x-1/2 w-72 rounded-xl p-5 shadow-2xl border ${card.deck === "chance" ? "bg-orange-100 border-orange-300 text-orange-950" : "bg-sky-100 border-sky-300 text-sky-950"}`}
        >
          <div className="text-xs uppercase tracking-widest opacity-70 mb-2">{card.deck === "chance" ? "Chance" : "Community Chest"}</div>
          <div className="font-medium leading-snug">{card.text}</div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

/** Pending trade banner for spectators / the proposer. */
export function TradeBanner() {
  const { state, config } = useGame();
  const me = useAuth((s) => s.user);
  const t = state?.trade;
  if (!t || !state || !config) return null;
  const name = (id: string) => state.players.find((p) => p.id === id)?.name ?? id;
  const side = (s: { cash: number; properties: number[]; jailCards: number }) =>
    [s.cash ? `$${s.cash}` : "", ...s.properties.map((i) => config.spaces[i].name), s.jailCards ? `${s.jailCards} jail card(s)` : ""].filter(Boolean).join(", ") || "nothing";
  return (
    <div className={`bg-slate-900/90 border rounded-xl px-4 py-2 text-sm ${t.toId === me?.id ? "border-emerald-400" : "border-slate-700"}`}>
      <span className="font-medium">{name(t.fromId)}</span> offers <span className="text-emerald-300">{side(t.give)}</span> to <span className="font-medium">{name(t.toId)}</span> for{" "}
      <span className="text-amber-300">{side(t.receive)}</span>
    </div>
  );
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

import { useEffect, useRef, useState } from "react";
import { useGame } from "@/store/game";
import { useAuth } from "@/store/auth";
import { seatColors } from "@/lib/ui";

/** Cash counter that ticks toward the animated balance. */
function Counter({ value }: { value: number }) {
  const [shown, setShown] = useState(value);
  const raf = useRef(0);
  useEffect(() => {
    cancelAnimationFrame(raf.current);
    const start = shown;
    const t0 = performance.now();
    const dur = 450;
    const step = (t: number) => {
      const k = Math.min(1, (t - t0) / dur);
      setShown(Math.round(start + (value - start) * (1 - Math.pow(1 - k, 3))));
      if (k < 1) raf.current = requestAnimationFrame(step);
    };
    raf.current = requestAnimationFrame(step);
    return () => cancelAnimationFrame(raf.current);
  }, [value]); // eslint-disable-line react-hooks/exhaustive-deps
  return <span className="tabular-nums">${shown.toLocaleString()}</span>;
}

export function PlayersRail() {
  const { state, seats, cash, waitingOn } = useGame();
  const me = useAuth((s) => s.user);
  if (!state) return null;
  const current = state.players[state.turn.playerIdx]?.id;
  return (
    <div className="flex flex-col gap-1.5 w-56">
      {state.players.map((p, i) => {
        const seat = seats.find((s) => s.playerId === p.id);
        const owned = state.spaces.filter((s) => s.ownerId === p.id).length;
        const waiting = waitingOn.includes(p.id);
        return (
          <div
            key={p.id}
            className={`rounded-lg px-3 py-2 bg-slate-900/80 border ${waiting ? "border-emerald-400/70" : p.id === current ? "border-slate-600" : "border-slate-800"} ${p.bankrupt ? "opacity-40" : ""}`}
          >
            <div className="flex items-center gap-2">
              <span className="w-2.5 h-2.5 rounded-full shrink-0" style={{ background: seatColors[i % seatColors.length] }} />
              <span className="font-medium truncate">
                {p.name}
                {p.id === me?.id && <span className="text-slate-500"> (you)</span>}
              </span>
              <span className="flex-1" />
              {p.isBot || seat?.isBot ? (
                <span className="text-[10px] uppercase text-slate-500">bot</span>
              ) : (
                <span className={`w-1.5 h-1.5 rounded-full ${seat?.connected ? "bg-emerald-400" : "bg-slate-600"}`} title={seat?.connected ? "online" : "offline"} />
              )}
            </div>
            <div className="flex items-center gap-2 text-sm text-slate-300 mt-0.5">
              <span className={p.bankrupt ? "line-through" : ""}>
                <Counter value={cash[p.id] ?? p.cash} />
              </span>
              <span className="text-slate-500">· {owned} deeds</span>
              {p.inJail && <span className="text-amber-300 text-xs">in jail</span>}
              {p.jailCards.length > 0 && <span className="text-xs text-slate-500">🎟 {p.jailCards.length}</span>}
              {p.bankrupt && <span className="text-rose-400 text-xs">bankrupt</span>}
            </div>
          </div>
        );
      })}
    </div>
  );
}

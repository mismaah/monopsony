import { useEffect, useRef, useState, type CSSProperties } from "react";
import { useGame } from "@/store/game";
import { useAuth } from "@/store/auth";
import { seatColors } from "@/lib/ui";
import { api } from "@/api/http";
import { T } from "@/anim/timing";

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

/** Floating +$/-$ amounts next to a player's name; newest enters from the right, oldest leaves to the left. */
function CashPops({ playerId }: { playerId: string }) {
  const pops = useGame((s) => s.cashPops);
  const mine = pops.filter((p) => p.playerId === playerId);
  if (!mine.length) return null;
  return (
    <span className="flex items-center min-w-0 overflow-hidden text-xs font-semibold tabular-nums" aria-live="polite">
      {mine.map((p) => (
        <span
          key={p.id}
          className={`cash-pop whitespace-nowrap ${p.delta >= 0 ? "text-emerald-300" : "text-rose-300"}`}
          style={{ "--cash-pop-hold": `${T.cashPop}ms` } as CSSProperties}
        >
          {p.delta >= 0 ? "+" : "−"}${Math.abs(p.delta).toLocaleString()}
        </span>
      ))}
    </span>
  );
}

/**
 * The seated players. On a wide screen this is a column of cards down the
 * left edge; on a phone it becomes a single swipeable row of compact chips
 * so it costs one line of the board instead of a third of the screen.
 */
export function PlayersRail({ compact = false }: { compact?: boolean }) {
  const { state, seats, cash, waitingOn, gameId, setError } = useGame();
  const me = useAuth((s) => s.user);
  const [confirmKick, setConfirmKick] = useState<string | null>(null);
  if (!state) return null;
  const current = state.players[state.turn.playerIdx]?.id;
  const isHost = !!seats.find((s) => s.playerId === me?.id)?.host;
  const kick = async (playerId: string) => {
    setConfirmKick(null);
    try {
      await api("DELETE", `/api/games/${gameId}/seats/${playerId}`);
    } catch (e) {
      setError((e as Error).message);
    }
  };

  const border = (p: { id: string; bankrupt: boolean }) =>
    waitingOn.includes(p.id) ? "border-emerald-400/70" : p.id === current ? "border-slate-600" : "border-slate-800";

  if (compact) {
    return (
      <div className="flex gap-1.5 overflow-x-auto no-scrollbar -mx-1 px-1 py-0.5">
        {state.players.map((p, i) => {
          const seat = seats.find((s) => s.playerId === p.id);
          const owned = state.spaces.filter((s) => s.ownerId === p.id).length;
          const kickable = isHost && p.id !== me?.id && !p.bankrupt && !seat?.isBot && state.turn.phase !== "game_over";
          if (kickable && confirmKick === p.id) {
            return (
              <div key={p.id} className="shrink-0 rounded-lg px-2 py-1 bg-slate-900/95 border border-rose-500/60 text-xs flex items-center gap-1.5">
                <span className="text-rose-300">Remove {p.name}?</span>
                <button type="button" className="px-1.5 py-1 rounded bg-rose-600 text-white" onClick={() => void kick(p.id)}>
                  Yes
                </button>
                <button type="button" className="px-1.5 py-1 rounded bg-slate-700 text-slate-200" onClick={() => setConfirmKick(null)}>
                  No
                </button>
              </div>
            );
          }
          return (
            <div
              key={p.id}
              className={`shrink-0 rounded-lg px-2 py-1 bg-slate-900/85 backdrop-blur-sm border ${border(p)} ${p.bankrupt ? "opacity-40" : ""}`}
            >
              <div className="flex items-center gap-1.5 text-xs leading-tight">
                <span className="w-2 h-2 rounded-full shrink-0" style={{ background: seatColors[i % seatColors.length] }} />
                <span className="font-medium max-w-22 truncate">{p.id === me?.id ? "You" : p.name}</span>
                {(p.isBot || seat?.isBot) && <span className="text-[9px] uppercase text-slate-500">bot</span>}
                {!p.isBot && !seat?.isBot && !seat?.connected && <span className="w-1.5 h-1.5 rounded-full bg-slate-600" title="offline" />}
                {kickable && (
                  <button
                    type="button"
                    className="text-slate-600 active:text-rose-300 leading-none"
                    aria-label={`Remove ${p.name}`}
                    onClick={() => setConfirmKick(p.id)}
                  >
                    ✕
                  </button>
                )}
              </div>
              <div className="flex items-center gap-1 text-xs mt-0.5">
                <span className={`font-medium ${p.bankrupt ? "line-through text-slate-500" : "text-slate-200"}`}>
                  <Counter value={cash[p.id] ?? p.cash} />
                </span>
                <span className="text-slate-500">·{owned}</span>
                {p.inJail && <span className="text-amber-300" title="in jail">⛓</span>}
                {p.jailCards.length > 0 && <span className="text-slate-500">🎟{p.jailCards.length}</span>}
                <CashPops playerId={p.id} />
              </div>
            </div>
          );
        })}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-1.5 w-56">
      {state.players.map((p, i) => {
        const seat = seats.find((s) => s.playerId === p.id);
        const owned = state.spaces.filter((s) => s.ownerId === p.id).length;
        // The host may hand any other human's seat to a bot.
        const kickable = isHost && p.id !== me?.id && !p.bankrupt && !seat?.isBot && state.turn.phase !== "game_over";
        return (
          <div key={p.id} className={`rounded-lg px-3 py-2 bg-slate-900/80 border ${border(p)} ${p.bankrupt ? "opacity-40" : ""}`}>
            <div className="flex items-center gap-2">
              <span className="w-2.5 h-2.5 rounded-full shrink-0" style={{ background: seatColors[i % seatColors.length] }} />
              <span className="font-medium truncate">
                {p.name}
                {p.id === me?.id && <span className="text-slate-500"> (you)</span>}
              </span>
              <CashPops playerId={p.id} />
              <span className="flex-1" />
              {p.isBot || seat?.isBot ? (
                <span className="text-[10px] uppercase text-slate-500">bot</span>
              ) : (
                <span className={`w-1.5 h-1.5 rounded-full ${seat?.connected ? "bg-emerald-400" : "bg-slate-600"}`} title={seat?.connected ? "online" : "offline"} />
              )}
              {kickable && confirmKick !== p.id && (
                <button
                  type="button"
                  className="ml-1 text-slate-600 hover:text-rose-300 text-xs leading-none"
                  title={`Remove ${p.name} (a bot takes over their seat)`}
                  onClick={() => setConfirmKick(p.id)}
                >
                  ✕
                </button>
              )}
            </div>
            {confirmKick === p.id && (
              <div className="flex items-center gap-1 mt-1 text-xs">
                <span className="text-rose-300 flex-1">Remove {p.name}? A bot takes the seat.</span>
                <button type="button" className="px-1.5 py-0.5 rounded bg-rose-600 hover:bg-rose-500 text-white" onClick={() => void kick(p.id)}>
                  Remove
                </button>
                <button type="button" className="px-1.5 py-0.5 rounded hover:bg-slate-800 text-slate-300" onClick={() => setConfirmKick(null)}>
                  Keep
                </button>
              </div>
            )}
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

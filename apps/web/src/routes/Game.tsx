import { useEffect, useRef, useState } from "react";
import { Scene } from "@/game3d/Scene";
import { PlayersRail } from "@/hud/PlayersRail";
import { ActionBar } from "@/hud/ActionBar";
import { PropertyPanel } from "@/hud/PropertyPanel";
import { TradeDialog } from "@/hud/TradeDialog";
import { RaiseFundsDialog } from "@/hud/RaiseFundsDialog";
import { useAuth } from "@/store/auth";
import { CardOverlay, GameOverOverlay, KickedOverlay, LogPanel, TradeBanner } from "@/hud/Overlays";
import { useGame } from "@/store/game";
import { socket } from "@/api/ws";
import { AdSlot } from "@/hud/AdSlot";
import { SoundControl } from "@/hud/SoundControl";
import { sfx } from "@/audio/sfx";

/** The in-game screen: full-bleed 3D canvas with HUD panels layered on top. */
export default function GamePage() {
  const [follow, setFollow] = useState(true);
  const [trade, setTrade] = useState(false);
  const [raise, setRaise] = useState(false);
  const lobby = useGame((s) => s.lobby);
  useEffect(() => sfx.install(), []);

  // Pop the raise-funds panel once per debt, after the landing animation has
  // played out; "Hide" keeps it away until the next debt. It closes itself
  // when the debt is gone.
  const me = useAuth((s) => s.user?.id);
  const phase = useGame((s) => s.state?.turn.phase);
  const animating = useGame((s) => s.animating);
  const inDebt = useGame((s) => !!me && (s.state?.debts ?? []).some((d) => d.debtorId === me));
  const raising = phase === "raising_funds" && inDebt;
  const autoOpened = useRef(false);
  useEffect(() => {
    if (!raising) {
      autoOpened.current = false;
      setRaise(false);
    } else if (!animating && !autoOpened.current) {
      autoOpened.current = true;
      setRaise(true);
    }
  }, [raising, animating]);

  return (
    <div className="relative h-full overflow-hidden select-none">
      <Scene follow={follow} />

      {/* top-left: players */}
      <div className="absolute left-3 top-3 z-10">
        <PlayersRail />
      </div>

      {/* top-right: table info + camera toggle */}
      <div className="absolute right-3 top-3 z-10 flex flex-col items-end gap-2">
        <div className="bg-slate-900/80 border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-slate-300 flex items-center gap-3">
          <span className="font-medium text-slate-100">{lobby?.name}</span>
          <label className="flex items-center gap-1 cursor-pointer">
            <input type="checkbox" checked={follow} onChange={(e) => setFollow(e.target.checked)} /> follow
          </label>
          <SoundControl />
          <ConnectionDot />
        </div>
        <PropertyPanel />
      </div>

      {/* bottom-centre: actions */}
      <div className="absolute left-1/2 bottom-4 -translate-x-1/2 z-10 flex flex-col items-center gap-2">
        <TradeBanner />
        <ActionBar onTrade={() => setTrade(true)} onRaiseFunds={() => setRaise(true)} />
      </div>

      {/* bottom-right: log/chat */}
      <div className="absolute right-3 bottom-4 z-10">
        <LogPanel />
      </div>

      {/* bottom-left: ad slot for the free tier (provider from /api/config) */}
      <div className="absolute left-3 bottom-4 z-10">
        <AdSlot placement="game" />
      </div>

      <CardOverlay />
      <GameOverOverlay />
      <KickedOverlay />
      {raise && !trade && <RaiseFundsDialog onClose={() => setRaise(false)} onTrade={() => setTrade(true)} />}
      {trade && <TradeDialog onClose={() => setTrade(false)} />}
    </div>
  );
}

function ConnectionDot() {
  const [status, setStatus] = useState(socket.status);
  socket.onStatus = setStatus;
  return <span title={status} className={`w-2 h-2 rounded-full ${status === "open" ? "bg-emerald-400" : status === "connecting" ? "bg-amber-400 animate-pulse" : "bg-rose-500"}`} />;
}

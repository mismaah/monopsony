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
import { useIsCompact, useIsShort } from "@/lib/responsive";

/** The in-game screen: full-bleed 3D canvas with HUD panels layered on top. */
export default function GamePage() {
  const [follow, setFollow] = useState(true);
  const [trade, setTrade] = useState(false);
  const [raise, setRaise] = useState(false);
  const lobby = useGame((s) => s.lobby);
  const compact = useIsCompact();
  const short = useIsShort();
  // A deed card or a live trade offer already fills the bottom strip; the ad
  // waits its turn rather than pushing the action bar off screen. A sideways
  // phone has no spare height for it at all.
  const selected = useGame((s) => s.selectedSpace);
  const offer = useGame((s) => s.state?.trade);
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

  const dialogs = (
    <>
      <CardOverlay />
      <GameOverOverlay />
      <KickedOverlay />
      {raise && !trade && <RaiseFundsDialog onClose={() => setRaise(false)} onTrade={() => setTrade(true)} />}
      {trade && <TradeDialog onClose={() => setTrade(false)} />}
    </>
  );

  // Phone layout: the board keeps the middle band of the screen and the HUD
  // lives in two full-width strips, top and bottom, instead of four floating
  // panels that would overlap each other at this width.
  if (compact) {
    return (
      <div className="relative h-full overflow-hidden select-none">
        <Scene follow={follow} />

        <div className="absolute inset-x-0 top-0 z-10 flex items-start gap-2 p-2 pad-safe-top">
          <div className="flex-1 min-w-0">
            <PlayersRail compact />
          </div>
          <div className="shrink-0 flex items-center gap-1 bg-slate-900/85 backdrop-blur-sm border border-slate-800 rounded-lg px-1.5 py-1 text-xs text-slate-300">
            <FollowToggle follow={follow} onChange={setFollow} compact />
            <SoundControl compact />
            <ConnectionDot />
          </div>
        </div>

        <div className="absolute inset-x-0 bottom-0 z-10 mx-auto w-full sm:max-w-lg flex flex-col items-center gap-2 p-2 pad-safe-bottom max-h-[80dvh] overflow-y-auto overscroll-contain no-scrollbar">
          <TradeBanner />
          <PropertyPanel />
          <LogPanel compact />
          {selected === null && !offer && !short && <AdSlot placement="game" />}
          <ActionBar onTrade={() => setTrade(true)} onRaiseFunds={() => setRaise(true)} />
        </div>

        {dialogs}
      </div>
    );
  }

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
          <FollowToggle follow={follow} onChange={setFollow} />
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

      {dialogs}
    </div>
  );
}

/** Camera-follow switch: a checkbox with room for a label, an icon without. */
function FollowToggle({ follow, onChange, compact = false }: { follow: boolean; onChange: (v: boolean) => void; compact?: boolean }) {
  if (compact) {
    return (
      <button
        type="button"
        aria-pressed={follow}
        aria-label={follow ? "Stop following the active player" : "Follow the active player"}
        title={follow ? "Camera follows the active player" : "Free camera"}
        className={`px-1 py-0.5 rounded ${follow ? "text-emerald-300" : "text-slate-500"}`}
        onClick={() => onChange(!follow)}
      >
        ◎
      </button>
    );
  }
  return (
    <label className="flex items-center gap-1 cursor-pointer">
      <input type="checkbox" checked={follow} onChange={(e) => onChange(e.target.checked)} /> follow
    </label>
  );
}

function ConnectionDot() {
  const [status, setStatus] = useState(socket.status);
  socket.onStatus = setStatus;
  return <span title={status} className={`w-2 h-2 rounded-full ${status === "open" ? "bg-emerald-400" : status === "connecting" ? "bg-amber-400 animate-pulse" : "bg-rose-500"}`} />;
}

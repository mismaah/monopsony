import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import type { Lobby } from "@monopsony/protocol";
import { api } from "@/api/http";
import { useAuth } from "@/store/auth";
import { Button, Card } from "@/lib/ui";
import { Mascot } from "@/brand";
import { JOIN_PATH, clearPendingInvite, normalizeInviteCode, rememberInvite } from "@/invite/code";

/**
 * The landing page for a shared invite link. Signed-out visitors are sent to
 * the front door first and come back here; everyone else is seated and
 * forwarded to the table.
 */
export default function JoinPage() {
  const { code: raw = "" } = useParams();
  const code = normalizeInviteCode(raw);
  const nav = useNavigate();
  const { user, ready } = useAuth();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!ready) return;
    if (!code) {
      setError("That invite link is missing its code.");
      return;
    }
    if (!user) {
      rememberInvite(code);
      nav("/", { replace: true, state: { from: `${JOIN_PATH}/${code}` } });
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const res = await api<{ game: Lobby }>("POST", "/api/games/join-code", { code });
        clearPendingInvite();
        if (!cancelled) nav(`/game/${res.game.gameId}`, { replace: true });
      } catch (e) {
        clearPendingInvite();
        if (!cancelled) setError((e as Error).message);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [code, ready, user, nav]);

  if (!error) {
    return (
      <div className="h-full grid place-items-center text-slate-400">
        <div className="animate-pulse">Taking your seat…</div>
      </div>
    );
  }

  return (
    <div className="h-full grid place-items-center p-4">
      <Card className="max-w-sm text-center space-y-3">
        <Mascot size={96} pose="plain" className="mx-auto" />
        <h1 className="text-lg font-semibold">Could not join that table</h1>
        <p className="text-sm text-slate-400">{error}</p>
        {code && <p className="text-xs text-slate-500 font-mono">{code}</p>}
        <Link to="/lobby">
          <Button className="w-full">Back to the lobby</Button>
        </Link>
      </Card>
    </div>
  );
}

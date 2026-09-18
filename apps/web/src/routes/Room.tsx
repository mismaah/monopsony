import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api } from "@/api/http";
import { useAuth } from "@/store/auth";
import { useGame } from "@/store/game";
import { Button, Card, seatColors } from "@/lib/ui";
import GamePage from "./Game";

/**
 * RoomPage subscribes to the game over the socket. Before the game starts it
 * shows the lobby (seats, bots, invite code); afterwards it renders the 3D game.
 */
export default function RoomPage() {
  const { id = "" } = useParams();
  const nav = useNavigate();
  const user = useAuth((s) => s.user);
  const { lobby, state, open, close } = useGame();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    open(id);
    return () => close();
  }, [id, open, close]);

  if (state) return <GamePage />;
  if (!lobby) {
    return (
      <div className="h-full grid place-items-center text-slate-400">
        <div className="animate-pulse">Joining table…</div>
      </div>
    );
  }

  const me = lobby.seats.find((s) => s.playerId === user?.id);
  const isHost = !!me?.host;
  const call = async (fn: () => Promise<unknown>) => {
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError((e as Error).message);
    }
  };

  return (
    <div className="max-w-3xl mx-auto p-4 space-y-4">
      <Card>
        <div className="flex items-start gap-4">
          <div className="flex-1">
            <h1 className="text-2xl font-bold">{lobby.name}</h1>
            <p className="text-sm text-slate-400">
              {lobby.visibility} · {lobby.seats.length}/{lobby.maxPlayers} seats · {lobby.turnSeconds}s per decision
            </p>
            {lobby.inviteCode && (
              <p className="mt-2 text-sm">
                Invite code: <code className="bg-slate-800 px-2 py-0.5 rounded font-mono text-emerald-300">{lobby.inviteCode}</code>
              </p>
            )}
          </div>
          {lobby.status === "finished" && <span className="text-amber-300">Finished</span>}
        </div>
      </Card>

      <Card title="Seats">
        <ul className="space-y-2">
          {lobby.seats.map((s, i) => (
            <li key={s.playerId} className="flex items-center gap-3">
              <span className="w-3 h-3 rounded-full" style={{ background: seatColors[i % seatColors.length] }} />
              <span className="flex-1">
                {s.name}
                {s.host && <span className="ml-2 text-xs text-amber-300">host</span>}
                {s.isBot && <span className="ml-2 text-xs text-slate-400">bot</span>}
                {!s.isBot && !s.connected && <span className="ml-2 text-xs text-slate-500">offline</span>}
              </span>
              {isHost && s.playerId !== user?.id && (
                <Button variant="ghost" onClick={() => call(() => api("DELETE", `/api/games/${id}/seats/${s.playerId}`))}>
                  Remove
                </Button>
              )}
            </li>
          ))}
          {Array.from({ length: Math.max(0, lobby.maxPlayers - lobby.seats.length) }).map((_, i) => (
            <li key={`empty-${i}`} className="text-slate-600 text-sm pl-6">
              empty seat
            </li>
          ))}
        </ul>
      </Card>

      <div className="flex flex-wrap gap-2 items-center">
        {!me && (
          <Button onClick={() => call(() => api("POST", `/api/games/${id}/join`))} disabled={lobby.seats.length >= lobby.maxPlayers}>
            Take a seat
          </Button>
        )}
        {isHost && (
          <>
            {(["balanced", "aggressive", "cautious"] as const).map((p) => (
              <Button
                key={p}
                variant="secondary"
                disabled={lobby.seats.length >= lobby.maxPlayers}
                onClick={() => call(() => api("POST", `/api/games/${id}/bots`, { profile: p }))}
              >
                + {p} bot
              </Button>
            ))}
            <Button disabled={lobby.seats.length < 2} onClick={() => call(() => api("POST", `/api/games/${id}/start`))}>
              Start game
            </Button>
          </>
        )}
        {me && (
          <Button
            variant="ghost"
            onClick={() =>
              call(async () => {
                await api("POST", `/api/games/${id}/leave`);
                nav("/lobby");
              })
            }
          >
            Leave
          </Button>
        )}
        {!isHost && me && <span className="text-sm text-slate-400">Waiting for the host to start…</span>}
      </div>
      {error && <p className="text-rose-400 text-sm">{error}</p>}
    </div>
  );
}

import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import type { Lobby } from "@monopsony/protocol";
import { api, ApiError } from "@/api/http";
import { useAuth } from "@/store/auth";
import { Button, Card, Input } from "@/lib/ui";
import { AdSlot } from "@/hud/AdSlot";

interface MyGame {
  id: string;
  name: string;
  status: string;
  winnerId?: string;
  players: number;
  updatedAt: number;
}

export default function LobbyPage() {
  const { caps, user } = useAuth();
  const nav = useNavigate();
  const [games, setGames] = useState<Lobby[]>([]);
  const [mine, setMine] = useState<MyGame[]>([]);
  const [name, setName] = useState("");
  const [visibility, setVisibility] = useState<"public" | "private">("public");
  const [maxPlayers, setMaxPlayers] = useState(4);
  const [turnSeconds, setTurnSeconds] = useState(60);
  const [houseRules, setHouseRules] = useState({ freeParkingJackpot: false, auctionsEnabled: true, doubleSalaryOnGoLanding: false, noRentInJail: false, turnLimit: 0 });
  const [code, setCode] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function refresh() {
    const [pub, my] = await Promise.all([
      api<{ games: Lobby[] | null }>("GET", "/api/games"),
      api<{ games: MyGame[] }>("GET", "/api/games/mine"),
    ]);
    setGames(pub.games ?? []);
    setMine(my.games ?? []);
  }

  useEffect(() => {
    void refresh();
    const t = setInterval(() => void refresh(), 5000);
    return () => clearInterval(t);
  }, []);

  async function create() {
    setError(null);
    try {
      const body: Record<string, unknown> = { name, visibility, maxPlayers, turnSeconds };
      if (caps?.houseRules) body.rules = { ...houseRules, startCash: 0 };
      const res = await api<{ game: Lobby }>("POST", "/api/games", body);
      nav(`/game/${res.game.gameId}`);
    } catch (e) {
      setError(e instanceof ApiError && e.status === 402 ? `${e.message} — upgrade to premium.` : (e as Error).message);
    }
  }

  async function join(id: string) {
    setError(null);
    try {
      await api("POST", `/api/games/${id}/join`);
      nav(`/game/${id}`);
    } catch (e) {
      setError((e as Error).message);
    }
  }

  async function joinCode() {
    setError(null);
    try {
      const res = await api<{ game: Lobby }>("POST", "/api/games/join-code", { code });
      nav(`/game/${res.game.gameId}`);
    } catch (e) {
      setError((e as Error).message);
    }
  }

  const premium = caps?.tier === "premium";

  return (
    <div className="max-w-5xl mx-auto p-4 grid md:grid-cols-[1fr_320px] gap-4">
      <div className="space-y-4">
        <Card title="Open tables">
          {games.length === 0 && <p className="text-slate-500 text-sm">No public games waiting. Start one!</p>}
          <ul className="divide-y divide-slate-800">
            {games.map((g) => (
              <li key={g.gameId} className="py-2 flex items-center gap-3">
                <div className="flex-1">
                  <div className="font-medium">{g.name}</div>
                  <div className="text-xs text-slate-400">
                    {g.seats.length}/{g.maxPlayers} seats · {g.turnSeconds}s turns
                    {g.rules.freeParkingJackpot && " · Free Parking jackpot"}
                    {!g.rules.auctionsEnabled && " · no auctions"}
                  </div>
                </div>
                <Button variant="secondary" onClick={() => join(g.gameId)} disabled={g.seats.length >= g.maxPlayers}>
                  Join
                </Button>
              </li>
            ))}
          </ul>
        </Card>
        <Card title="Your games">
          {mine.length === 0 && <p className="text-slate-500 text-sm">Nothing yet.</p>}
          <ul className="divide-y divide-slate-800">
            {mine.map((g) => (
              <li key={g.id} className="py-2 flex items-center gap-3">
                <div className="flex-1">
                  <div className="font-medium">{g.name}</div>
                  <div className="text-xs text-slate-400">
                    {g.status.replace("_", " ")} · {g.players} players
                    {g.status === "finished" && g.winnerId === user?.id && " · you won!"}
                  </div>
                </div>
                {g.status !== "finished" && (
                  <Button variant="secondary" onClick={() => nav(`/game/${g.id}`)}>
                    Open
                  </Button>
                )}
              </li>
            ))}
          </ul>
        </Card>
      </div>
      <div className="space-y-4">
        <Card title="New table">
          <div className="space-y-2">
            <Input placeholder="Table name" value={name} onChange={(e) => setName(e.target.value)} maxLength={40} />
            <label className="flex items-center justify-between text-sm">
              <span>Players</span>
              <select className="bg-slate-800 rounded px-2 py-1" value={maxPlayers} onChange={(e) => setMaxPlayers(+e.target.value)}>
                {[2, 3, 4, 5, 6, 7, 8].map((n) => (
                  <option key={n} value={n} disabled={n > (caps?.maxPlayersPerRoom ?? 4)}>
                    {n}
                    {n > (caps?.maxPlayersPerRoom ?? 4) ? " (premium)" : ""}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex items-center justify-between text-sm">
              <span>Turn timer</span>
              <select className="bg-slate-800 rounded px-2 py-1" value={turnSeconds} onChange={(e) => setTurnSeconds(+e.target.value)}>
                {[30, 60, 90, 120, 180].map((n) => (
                  <option key={n} value={n}>
                    {n}s
                  </option>
                ))}
              </select>
            </label>
            <label className="flex items-center justify-between text-sm">
              <span>Private (invite code)</span>
              <input
                type="checkbox"
                checked={visibility === "private"}
                disabled={!caps?.privateRooms}
                onChange={(e) => setVisibility(e.target.checked ? "private" : "public")}
              />
            </label>
            <fieldset className={`text-sm space-y-1 ${premium ? "" : "opacity-50"}`} disabled={!premium}>
              <legend className="text-xs uppercase text-slate-500 mt-2">House rules {premium ? "" : "(premium)"}</legend>
              {(
                [
                  ["freeParkingJackpot", "Free Parking jackpot"],
                  ["auctionsEnabled", "Auction declined properties"],
                  ["doubleSalaryOnGoLanding", "Double salary on Go"],
                  ["noRentInJail", "No rent while in jail"],
                ] as const
              ).map(([k, label]) => (
                <label key={k} className="flex items-center justify-between">
                  <span>{label}</span>
                  <input type="checkbox" checked={houseRules[k]} onChange={(e) => setHouseRules({ ...houseRules, [k]: e.target.checked })} />
                </label>
              ))}
              <label className="flex items-center justify-between">
                <span>Turn limit (0 = none)</span>
                <input
                  type="number"
                  className="bg-slate-800 rounded px-2 py-0.5 w-20"
                  min={0}
                  value={houseRules.turnLimit}
                  onChange={(e) => setHouseRules({ ...houseRules, turnLimit: +e.target.value })}
                />
              </label>
            </fieldset>
            <Button className="w-full" onClick={create}>
              Create table
            </Button>
          </div>
        </Card>
        <Card title="Join with a code">
          <div className="flex gap-2">
            <Input placeholder="ABCD-1234" value={code} onChange={(e) => setCode(e.target.value.toUpperCase())} />
            <Button variant="secondary" onClick={joinCode} disabled={code.length < 4}>
              Join
            </Button>
          </div>
        </Card>
        {error && <p className="text-rose-400 text-sm">{error}</p>}
        <AdSlot placement="lobby" />
      </div>
    </div>
  );
}

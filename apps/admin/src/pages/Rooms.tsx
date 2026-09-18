import { useEffect, useState } from "react";
import type { Lobby } from "@monopsony/protocol";
import { api } from "@/lib/http";
import { Btn, Panel } from "@/lib/ui";

export default function RoomsPage() {
  const [rooms, setRooms] = useState<Lobby[]>([]);
  const load = () => api<{ rooms: Lobby[] }>("GET", "/admin/api/rooms").then((r) => setRooms(r.rooms));
  useEffect(() => {
    void load();
    const t = setInterval(() => void load(), 5000);
    return () => clearInterval(t);
  }, []);
  return (
    <Panel title="Live rooms" actions={<Btn tone="secondary" onClick={load}>Refresh</Btn>}>
      <table className="w-full text-sm">
        <thead className="text-zinc-400">
          <tr>
            <th>Name</th>
            <th>Status</th>
            <th>Visibility</th>
            <th>Seats</th>
            <th>Config</th>
            <th>Node</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {rooms.map((r) => (
            <tr key={r.gameId} className="border-t border-zinc-800">
              <td>
                {r.name}
                <div className="text-xs text-zinc-500 font-mono">{r.gameId}</div>
              </td>
              <td>{r.status.replace("_", " ")}</td>
              <td>{r.visibility}{r.inviteCode ? ` (${r.inviteCode})` : ""}</td>
              <td>
                {r.seats.map((s) => (
                  <span key={s.playerId} className={`inline-block mr-2 ${s.connected ? "" : "text-zinc-500"}`}>
                    {s.name}{s.isBot ? " (bot)" : ""}
                  </span>
                ))}
              </td>
              <td className="text-xs text-zinc-400">{r.configId}</td>
              <td className={`text-xs font-mono ${r.node === "unowned" ? "text-amber-300" : "text-zinc-400"}`}>{r.node ?? "—"}</td>
              <td className="text-right">
                {r.status !== "finished" && (
                  <Btn tone="danger" onClick={async () => { if (confirm(`End ${r.name}?`)) { await api("POST", `/admin/api/rooms/${r.gameId}/end`); await load(); } }}>
                    Force end
                  </Btn>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {rooms.length === 0 && <p className="text-sm text-zinc-500">No live rooms.</p>}
    </Panel>
  );
}

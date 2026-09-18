import { useEffect, useState } from "react";
import { api } from "@/lib/http";
import { Btn, Notice, Panel } from "@/lib/ui";

interface User {
  id: string;
  email?: string;
  name: string;
  role: string;
  tier: string;
  guest: boolean;
  banned: boolean;
  createdAt: string;
}

export default function UsersPage() {
  const [q, setQ] = useState("");
  const [users, setUsers] = useState<User[]>([]);
  const [cosmetics, setCosmetics] = useState<{ id: string; name: string }[]>([]);
  const [grant, setGrant] = useState<Record<string, string>>({});
  const [msg, setMsg] = useState<{ kind: "ok" | "err"; text: string } | null>(null);

  const load = () => api<{ users: User[] }>("GET", `/admin/api/users?q=${encodeURIComponent(q)}`).then((r) => setUsers(r.users));
  useEffect(() => {
    void load();
    api<{ items: { id: string; name: string }[] }>("GET", "/admin/api/cosmetics").then((r) => setCosmetics(r.items));
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  async function patch(u: User, body: Record<string, unknown>) {
    try {
      await api("PATCH", `/admin/api/users/${u.id}`, body);
      setMsg({ kind: "ok", text: `${u.name} updated.` });
      await load();
    } catch (e) {
      setMsg({ kind: "err", text: (e as Error).message });
    }
  }

  return (
    <Panel
      title="Users"
      actions={
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void load();
          }}
          className="flex gap-2"
        >
          <input placeholder="Search name, email or id" value={q} onChange={(e) => setQ(e.target.value)} />
          <Btn tone="secondary">Search</Btn>
        </form>
      }
    >
      {msg && <div className="mb-3"><Notice kind={msg.kind} text={msg.text} /></div>}
      <table className="w-full text-sm">
        <thead className="text-zinc-400">
          <tr>
            <th>Name</th>
            <th>Email</th>
            <th>Tier</th>
            <th>Role</th>
            <th>Status</th>
            <th>Grant cosmetic</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {users.map((u) => (
            <tr key={u.id} className="border-t border-zinc-800">
              <td>
                {u.name}
                {u.guest && <span className="text-xs text-zinc-500 ml-1">guest</span>}
              </td>
              <td className="text-zinc-400">{u.email ?? "—"}</td>
              <td>
                <select value={u.tier} onChange={(e) => patch(u, { tier: e.target.value })}>
                  <option value="free">free</option>
                  <option value="premium">premium</option>
                </select>
              </td>
              <td>
                <select value={u.role} onChange={(e) => patch(u, { role: e.target.value })}>
                  <option value="player">player</option>
                  <option value="admin">admin</option>
                </select>
              </td>
              <td>{u.banned ? <span className="text-rose-400">banned</span> : <span className="text-emerald-300">active</span>}</td>
              <td>
                <div className="flex gap-1">
                  <select value={grant[u.id] ?? ""} onChange={(e) => setGrant({ ...grant, [u.id]: e.target.value })}>
                    <option value="">—</option>
                    {cosmetics.map((c) => (
                      <option key={c.id} value={c.id}>
                        {c.name}
                      </option>
                    ))}
                  </select>
                  <Btn
                    tone="secondary"
                    disabled={!grant[u.id]}
                    onClick={async () => {
                      await api("POST", `/admin/api/users/${u.id}/grant`, { cosmeticId: grant[u.id] });
                      setMsg({ kind: "ok", text: `Granted ${grant[u.id]} to ${u.name}.` });
                    }}
                  >
                    Grant
                  </Btn>
                </div>
              </td>
              <td className="text-right">
                <Btn tone={u.banned ? "secondary" : "danger"} onClick={() => patch(u, { banned: !u.banned })}>
                  {u.banned ? "Unban" : "Ban"}
                </Btn>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </Panel>
  );
}

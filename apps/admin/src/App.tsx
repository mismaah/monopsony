import { useEffect, useState, type FormEvent } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, refreshSession, setAccessToken } from "@/lib/http";
import ConfigsPage from "@/pages/Configs";
import PlansPage from "@/pages/Plans";
import UsersPage from "@/pages/Users";
import CosmeticsPage from "@/pages/Cosmetics";
import RoomsPage from "@/pages/Rooms";
import AuditPage from "@/pages/Audit";

interface Admin {
  id: string;
  name: string;
  email: string;
  role: string;
}

export default function App() {
  const [admin, setAdmin] = useState<Admin | null>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    (async () => {
      if (await refreshSession()) {
        try {
          const me = await api<{ user: Admin }>("GET", "/admin/api/me");
          setAdmin(me.user);
        } catch {
          /* not an admin */
        }
      }
      setReady(true);
    })();
  }, []);

  if (!ready) return <div className="p-8 text-zinc-400">Loading…</div>;
  if (!admin) return <Login onLogin={setAdmin} />;

  const link = ({ isActive }: { isActive: boolean }) => `block px-3 py-1.5 rounded ${isActive ? "bg-zinc-800 text-white" : "text-zinc-400 hover:text-white"}`;
  return (
    <div className="h-full flex">
      <aside className="w-52 border-r border-zinc-800 p-3 flex flex-col gap-1 shrink-0">
        <div className="font-bold text-emerald-400 px-3 py-2">Monopsony Admin</div>
        <NavLink to="/configs" className={link}>Game configs</NavLink>
        <NavLink to="/plans" className={link}>Plans & tiers</NavLink>
        <NavLink to="/users" className={link}>Users</NavLink>
        <NavLink to="/cosmetics" className={link}>Cosmetics</NavLink>
        <NavLink to="/rooms" className={link}>Live rooms</NavLink>
        <NavLink to="/audit" className={link}>Audit log</NavLink>
        <div className="flex-1" />
        <div className="text-xs text-zinc-500 px-3">{admin.email}</div>
        <button
          className="text-left px-3 py-1.5 text-zinc-400 hover:text-white text-sm"
          onClick={async () => {
            await api("POST", "/api/auth/logout").catch(() => {});
            setAccessToken(null);
            setAdmin(null);
          }}
        >
          Sign out
        </button>
      </aside>
      <main className="flex-1 min-w-0 overflow-auto p-6">
        <Routes>
          <Route path="/configs/*" element={<ConfigsPage />} />
          <Route path="/plans" element={<PlansPage />} />
          <Route path="/users" element={<UsersPage />} />
          <Route path="/cosmetics" element={<CosmeticsPage />} />
          <Route path="/rooms" element={<RoomsPage />} />
          <Route path="/audit" element={<AuditPage />} />
          <Route path="*" element={<Navigate to="/configs" replace />} />
        </Routes>
      </main>
    </div>
  );
}

function Login({ onLogin }: { onLogin: (a: Admin) => void }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      const s = await api<{ accessToken: string }>("POST", "/api/auth/login", { email, password });
      setAccessToken(s.accessToken);
      const me = await api<{ user: Admin }>("GET", "/admin/api/me");
      onLogin(me.user);
    } catch (err) {
      setError((err as Error).message === "admin role required" ? "This account is not an admin." : (err as Error).message);
    }
  }
  return (
    <div className="h-full grid place-items-center">
      <form onSubmit={submit} className="w-80 space-y-3 bg-zinc-900 border border-zinc-800 rounded-xl p-5">
        <h1 className="font-bold text-lg">Admin sign in</h1>
        <input className="w-full" type="email" placeholder="Email" value={email} onChange={(e) => setEmail(e.target.value)} required />
        <input className="w-full" type="password" placeholder="Password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        {error && <p className="text-rose-400 text-sm">{error}</p>}
        <button className="w-full bg-emerald-500 text-zinc-950 font-semibold rounded py-1.5">Sign in</button>
        <p className="text-xs text-zinc-500">Admins are accounts whose email is listed in MONOPSONY_ADMIN_EMAILS, or promoted by another admin.</p>
      </form>
    </div>
  );
}

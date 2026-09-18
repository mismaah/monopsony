import { useEffect, useState, type FormEvent } from "react";
import { api } from "@/api/http";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "@/store/auth";
import { Button, Card, Input } from "@/lib/ui";
import { Logo, Mascot, TAGLINE } from "@/brand";

type Mode = "guest" | "login" | "register";

export default function Home() {
  const { user, ready, guest, login, register } = useAuth();
  const nav = useNavigate();
  const loc = useLocation();
  const [mode, setMode] = useState<Mode>("guest");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [providers, setProviders] = useState<string[]>([]);
  useEffect(() => {
    api<{ providers: string[] }>("GET", "/api/auth/providers").then((r) => setProviders(r.providers ?? [])).catch(() => {});
  }, []);

  if (ready && user) return <Navigate to={(loc.state as { from?: string } | null)?.from ?? "/lobby"} replace />;

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      if (mode === "guest") await guest(name);
      else if (mode === "login") await login(email, password);
      else await register(email, password, name);
      nav("/lobby");
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="h-full grid place-items-center p-4">
      <div className="w-full max-w-md space-y-4">
        <div className="text-center">
          <Mascot size={132} className="mx-auto -mb-3" />
          <h1>
            <Logo className="text-4xl" markSize={40} />
          </h1>
          <p className="text-emerald-300 font-semibold mt-1">{TAGLINE}</p>
          <p className="text-slate-400">Buy, build, bankrupt your friends — in 3D.</p>
        </div>
        <Card>
          <div className="flex gap-1 mb-4 bg-slate-800 rounded-md p-1">
            {(["guest", "login", "register"] as Mode[]).map((m) => (
              <button
                key={m}
                onClick={() => setMode(m)}
                className={`flex-1 py-1 rounded text-sm capitalize ${mode === m ? "bg-slate-600" : "hover:bg-slate-700"}`}
              >
                {m === "guest" ? "Play as guest" : m}
              </button>
            ))}
          </div>
          <form onSubmit={submit} className="space-y-3">
            {mode !== "login" && (
              <Input placeholder="Display name" value={name} onChange={(e) => setName(e.target.value)} required minLength={2} maxLength={24} />
            )}
            {mode !== "guest" && (
              <>
                <Input type="email" placeholder="Email" value={email} onChange={(e) => setEmail(e.target.value)} required />
                <Input
                  type="password"
                  placeholder="Password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  minLength={8}
                />
              </>
            )}
            {error && <p className="text-rose-400 text-sm">{error}</p>}
            <Button className="w-full" disabled={busy}>
              {mode === "guest" ? "Jump in" : mode === "login" ? "Sign in" : "Create account"}
            </Button>
          </form>
          {providers.length > 0 && (
            <div className="mt-4 space-y-2">
              <div className="text-center text-xs text-slate-500">or continue with</div>
              <div className="flex gap-2">
                {providers.map((p) => (
                  <a key={p} href={`/api/auth/oauth/${p}/start`} className="flex-1 text-center py-1.5 rounded-md bg-slate-700 hover:bg-slate-600 text-sm capitalize">
                    {p}
                  </a>
                ))}
              </div>
            </div>
          )}
        </Card>
        <p className="text-center text-xs text-slate-500">
          Free tier: public rooms, up to 4 players, standard rules. Go premium for private rooms, 8 players, house rules and skins.
        </p>
      </div>
    </div>
  );
}

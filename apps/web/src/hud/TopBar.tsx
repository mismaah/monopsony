import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "@/store/auth";
import { Button } from "@/lib/ui";
import { LogoMark } from "@/brand";

export function TopBar() {
  const { user, caps, logout } = useAuth();
  const nav = useNavigate();
  return (
    <header className="flex items-center gap-4 px-4 h-12 border-b border-slate-800 bg-slate-950/90 backdrop-blur shrink-0">
      <Link to={user ? "/lobby" : "/"} className="flex items-center gap-2 font-black tracking-tight text-slate-100 hover:text-emerald-300">
        <LogoMark size={24} />
        MONOPSONY
      </Link>
      {user && (
        <nav className="flex gap-3 text-sm text-slate-300">
          <Link to="/lobby" className="hover:text-white">Tables</Link>
          <Link to="/shop" className="hover:text-white">Shop</Link>
          <Link to="/collection" className="hover:text-white">Collection</Link>
          <Link to="/account" className="hover:text-white">Account</Link>
        </nav>
      )}
      <div className="flex-1" />
      {user && (
        <>
          <span className="text-sm text-slate-300">
            {user.name}
            <span className={`ml-2 text-xs px-1.5 py-0.5 rounded ${caps?.tier === "premium" ? "bg-amber-500/20 text-amber-300" : "bg-slate-800 text-slate-400"}`}>
              {caps?.tier ?? "free"}
            </span>
          </span>
          <Button
            variant="ghost"
            onClick={async () => {
              await logout();
              nav("/");
            }}
          >
            Sign out
          </Button>
        </>
      )}
    </header>
  );
}

import { useEffect, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "@/store/auth";
import { Button } from "@/lib/ui";
import { LogoMark } from "@/brand";

const links = [
  ["/lobby", "Tables"],
  ["/shop", "Shop"],
  ["/collection", "Collection"],
  ["/account", "Account"],
] as const;

export function TopBar() {
  const { user, caps, logout } = useAuth();
  const nav = useNavigate();
  const loc = useLocation();
  const [menu, setMenu] = useState(false);
  // A route change closes the sheet, so tapping a link never leaves it open.
  useEffect(() => setMenu(false), [loc.pathname]);

  const tier = (
    <span className={`text-xs px-1.5 py-0.5 rounded ${caps?.tier === "premium" ? "bg-amber-500/20 text-amber-300" : "bg-slate-800 text-slate-400"}`}>
      {caps?.tier ?? "free"}
    </span>
  );

  return (
    <header className="relative flex items-center gap-3 sm:gap-4 px-3 sm:px-4 h-11 sm:h-12 border-b border-slate-800 bg-slate-950/90 backdrop-blur shrink-0 z-40">
      <Link to={user ? "/lobby" : "/"} className="flex items-center gap-2 font-black tracking-tight text-slate-100 hover:text-emerald-300">
        <LogoMark size={24} />
        <span className="text-sm sm:text-base">MONOPSONY</span>
      </Link>
      {user && (
        <nav className="hidden sm:flex gap-3 text-sm text-slate-300">
          {links.map(([to, label]) => (
            <Link key={to} to={to} className="hover:text-white">
              {label}
            </Link>
          ))}
        </nav>
      )}
      <div className="flex-1" />
      {user && (
        <>
          <span className="hidden sm:flex items-center gap-2 text-sm text-slate-300">
            <span className="max-w-[12rem] truncate">{user.name}</span>
            {tier}
          </span>
          <Button
            variant="ghost"
            className="hidden sm:block"
            onClick={async () => {
              await logout();
              nav("/");
            }}
          >
            Sign out
          </Button>
          <button
            type="button"
            className="sm:hidden -mr-1 p-2 rounded-md text-slate-300 hover:bg-slate-800"
            aria-label="Menu"
            aria-expanded={menu}
            onClick={() => setMenu((m) => !m)}
          >
            <span className="block w-5 border-t-2 border-current" />
            <span className="block w-5 border-t-2 border-current mt-1" />
            <span className="block w-5 border-t-2 border-current mt-1" />
          </button>
        </>
      )}

      {/* Mobile sheet. Rendered only while open so its links never duplicate
          the desktop nav for assistive tech (or for tests). */}
      {user && menu && (
        <>
          <button type="button" aria-label="Close menu" className="sm:hidden fixed inset-0 top-11 bg-black/50 cursor-default" onClick={() => setMenu(false)} />
          <div className="sm:hidden absolute right-2 top-full mt-1 w-56 rounded-xl border border-slate-800 bg-slate-900 shadow-2xl overflow-hidden">
            <div className="flex items-center gap-2 px-3 py-2 border-b border-slate-800 text-sm text-slate-300">
              <span className="flex-1 truncate">{user.name}</span>
              {tier}
            </div>
            {links.map(([to, label]) => (
              <Link key={to} to={to} className="block px-3 py-2.5 text-sm text-slate-200 hover:bg-slate-800">
                {label}
              </Link>
            ))}
            <button
              type="button"
              className="w-full text-left px-3 py-2.5 text-sm text-rose-300 border-t border-slate-800 hover:bg-slate-800"
              onClick={async () => {
                setMenu(false);
                await logout();
                nav("/");
              }}
            >
              Sign out
            </button>
          </div>
        </>
      )}
    </header>
  );
}

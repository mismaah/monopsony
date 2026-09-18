import { useEffect } from "react";
import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import { useAuth } from "@/store/auth";
import Home from "@/routes/Home";
import LobbyPage from "@/routes/Lobby";
import RoomPage from "@/routes/Room";
import ShopPage from "@/routes/Shop";
import AccountPage from "@/routes/Account";
import { TopBar } from "@/hud/TopBar";

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { user, ready } = useAuth();
  const loc = useLocation();
  if (!ready) return <Splash />;
  if (!user) return <Navigate to="/" state={{ from: loc.pathname }} replace />;
  return <>{children}</>;
}

function Splash() {
  return (
    <div className="h-full grid place-items-center text-slate-400">
      <div className="animate-pulse">Loading…</div>
    </div>
  );
}

export default function App() {
  const bootstrap = useAuth((s) => s.bootstrap);
  useEffect(() => {
    void bootstrap();
  }, [bootstrap]);

  return (
    <div className="h-full flex flex-col">
      <TopBar />
      <div className="flex-1 min-h-0">
        <Routes>
          <Route path="/" element={<Home />} />
          <Route
            path="/lobby"
            element={
              <RequireAuth>
                <LobbyPage />
              </RequireAuth>
            }
          />
          <Route
            path="/game/:id"
            element={
              <RequireAuth>
                <RoomPage />
              </RequireAuth>
            }
          />
          <Route
            path="/shop"
            element={
              <RequireAuth>
                <ShopPage />
              </RequireAuth>
            }
          />
          <Route
            path="/account"
            element={
              <RequireAuth>
                <AccountPage />
              </RequireAuth>
            }
          />
          <Route path="/oauth/done" element={<Navigate to="/lobby" replace />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </div>
    </div>
  );
}

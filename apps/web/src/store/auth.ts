import { create } from "zustand";
import { api, refreshSession, setAccessToken, getAccessToken, sessionUser } from "@/api/http";
import { socket } from "@/api/ws";
import { useCosmetics } from "@/store/cosmetics";

export interface User {
  id: string;
  email?: string;
  name: string;
  role: string;
  tier: string;
  guest: boolean;
}

export interface Caps {
  tier: string;
  maxPlayersPerRoom: number;
  privateRooms: boolean;
  houseRules: boolean;
  showAds: boolean;
  statsHistoryDays: number;
  maxConcurrentGames: number;
  cosmeticSlots: string[];
}

interface AuthState {
  user: User | null;
  caps: Caps | null;
  ready: boolean; // bootstrap finished
  bootstrap: () => Promise<void>;
  guest: (name: string) => Promise<void>;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, name: string) => Promise<void>;
  logout: () => Promise<void>;
  reloadMe: () => Promise<void>;
}

type SessionResponse = { accessToken: string; user: User };

export const useAuth = create<AuthState>((set, get) => ({
  user: null,
  caps: null,
  ready: false,
  async bootstrap() {
    if (await refreshSession()) {
      set({ user: sessionUser as User });
      await get().reloadMe();
      socket.connect();
    }
    set({ ready: true });
  },
  async reloadMe() {
    if (!getAccessToken()) return;
    const me = await api<{ user: User; caps: Caps }>("GET", "/api/me");
    set({ user: me.user, caps: me.caps });
    void useCosmetics.getState().load();
  },
  async guest(name) {
    const s = await api<SessionResponse>("POST", "/api/auth/guest", { name });
    setAccessToken(s.accessToken);
    set({ user: s.user });
    await get().reloadMe();
    socket.connect();
  },
  async login(email, password) {
    const s = await api<SessionResponse>("POST", "/api/auth/login", { email, password });
    setAccessToken(s.accessToken);
    set({ user: s.user });
    await get().reloadMe();
    socket.connect();
  },
  async register(email, password, name) {
    const s = await api<SessionResponse>("POST", "/api/auth/register", { email, password, name });
    setAccessToken(s.accessToken);
    set({ user: s.user });
    await get().reloadMe();
    socket.connect();
  },
  async logout() {
    await api("POST", "/api/auth/logout").catch(() => {});
    setAccessToken(null);
    socket.close();
    set({ user: null, caps: null });
  },
}));

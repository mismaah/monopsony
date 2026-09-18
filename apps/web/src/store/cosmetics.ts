import { create } from "zustand";
import { api } from "@/api/http";
import type { Manifests } from "@/game3d/skins";

/**
 * Manifests for every enabled cosmetic, keyed by item id. Loaded once per
 * session (and re-fetched when the shop changes something) so the scene
 * can render any loadout it sees at the table, including other players'
 * glTF-backed items.
 */
interface CosmeticsState {
  manifests: Manifests;
  loaded: boolean;
  load: (force?: boolean) => Promise<void>;
}

let inflight: Promise<void> | null = null;

export const useCosmetics = create<CosmeticsState>((set, get) => ({
  manifests: {},
  loaded: false,
  async load(force = false) {
    if (get().loaded && !force) return;
    if (inflight) return inflight;
    inflight = (async () => {
      try {
        const res = await api<{ items: Manifests }>("GET", "/api/cosmetics/manifests");
        set({ manifests: res.items ?? {}, loaded: true });
      } catch {
        // Built-in looks still work without manifests; try again next time.
      } finally {
        inflight = null;
      }
    })();
    return inflight;
  },
}));

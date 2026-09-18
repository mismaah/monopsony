import { create } from "zustand";
import { persist } from "zustand/middleware";

/** Client-only preferences, persisted in localStorage. */
interface Settings {
  sfx: boolean;
  volume: number; // 0..1
  setSfx: (on: boolean) => void;
  setVolume: (v: number) => void;
}

export const useSettings = create<Settings>()(
  persist(
    (set) => ({
      sfx: true,
      volume: 0.7,
      setSfx: (sfx) => set({ sfx }),
      setVolume: (v) => set({ volume: Math.min(1, Math.max(0, v)) }),
    }),
    { name: "monopsony.settings", partialize: (s) => ({ sfx: s.sfx, volume: s.volume }) },
  ),
);

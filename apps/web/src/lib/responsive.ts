import { useEffect, useState } from "react";

/** Subscribes to a media query and re-renders when it flips. */
export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() => (typeof window === "undefined" ? false : window.matchMedia(query).matches));
  useEffect(() => {
    const mq = window.matchMedia(query);
    const onChange = () => setMatches(mq.matches);
    onChange();
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, [query]);
  return matches;
}

/**
 * Not enough room for the corner-panel HUD: the panels stack into top and
 * bottom strips instead. Narrow covers portrait phones (Tailwind's `sm`
 * breakpoint, so class- and structure-level rules agree); short covers a
 * phone turned landscape, where the corners would collide vertically.
 */
export function useIsCompact(): boolean {
  return useMediaQuery("(max-width: 639px), (max-height: 520px)");
}

/** A phone held sideways: too short for a stacked HUD to also carry an ad. */
export function useIsShort(): boolean {
  return useMediaQuery("(max-height: 520px)");
}

/** Touch-first device: used to trade render resolution for frame rate. */
export function useIsTouch(): boolean {
  return useMediaQuery("(pointer: coarse)");
}

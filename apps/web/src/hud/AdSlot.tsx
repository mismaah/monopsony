import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useAuth } from "@/store/auth";
import { Mascot } from "@/brand";

/**
 * AdSlot renders an ad for free-tier users and nothing for anyone else.
 * Which provider fills it comes from the server (`GET /api/config`) so it
 * can be changed without a rebuild:
 *
 *   none     - render nothing
 *   house    - built-in "go premium" promo (default; also the fallback
 *              when a provider is misconfigured)
 *   adsense  - Google AdSense (needs client + slot ids)
 */
export interface AdsConfig {
  provider: "none" | "house" | "adsense";
  client?: string;
  slot?: string;
  nonPersonalized?: boolean;
}

export type Placement = "game" | "lobby" | "shop";

let configPromise: Promise<AdsConfig> | null = null;

/** The ads section of /api/config, fetched once per page load. */
export function loadAdsConfig(): Promise<AdsConfig> {
  if (!configPromise) {
    configPromise = fetch("/api/config")
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(String(r.status)))))
      .then((j) => (j.ads ?? { provider: "house" }) as AdsConfig)
      .catch(() => ({ provider: "house" }) as AdsConfig);
  }
  return configPromise;
}

export function useAdsConfig(): AdsConfig | null {
  const [cfg, setCfg] = useState<AdsConfig | null>(null);
  useEffect(() => {
    let live = true;
    void loadAdsConfig().then((c) => live && setCfg(c));
    return () => {
      live = false;
    };
  }, []);
  return cfg;
}

const sizes: Record<Placement, string> = {
  game: "w-full h-14 sm:w-64 sm:h-16",
  lobby: "w-full min-h-[250px]",
  shop: "w-full min-h-[90px]",
};

export function AdSlot({ placement, className = "" }: { placement: Placement; className?: string }) {
  const caps = useAuth((s) => s.caps);
  const cfg = useAdsConfig();
  if (!caps?.showAds || !cfg || cfg.provider === "none") return null;
  const box = `${sizes[placement]} rounded-lg overflow-hidden ${className}`;
  if (cfg.provider === "adsense" && cfg.client && cfg.slot) {
    return (
      <div className={box} data-testid="ad-slot" data-provider="adsense" data-placement={placement}>
        <AdSense client={cfg.client} slot={cfg.slot} nonPersonalized={!!cfg.nonPersonalized} placement={placement} />
      </div>
    );
  }
  return (
    <div className={box} data-testid="ad-slot" data-provider="house" data-placement={placement}>
      <HouseAd placement={placement} />
    </div>
  );
}

// ---- Google AdSense --------------------------------------------------------------

declare global {
  interface Window {
    adsbygoogle?: unknown[] & { requestNonPersonalizedAds?: number; pauseAdRequests?: number };
  }
}

let scriptLoaded = false;

function ensureAdSenseScript(client: string) {
  if (scriptLoaded || document.querySelector("script[data-monopsony-adsense]")) return;
  scriptLoaded = true;
  const s = document.createElement("script");
  s.async = true;
  s.crossOrigin = "anonymous";
  s.src = `https://pagead2.googlesyndication.com/pagead/js/adsbygoogle.js?client=${encodeURIComponent(client)}`;
  s.dataset.monopsonyAdsense = "1";
  document.head.appendChild(s);
}

function AdSense({ client, slot, nonPersonalized, placement }: { client: string; slot: string; nonPersonalized: boolean; placement: Placement }) {
  const ref = useRef<HTMLModElement>(null);
  useEffect(() => {
    ensureAdSenseScript(client);
    const ins = ref.current;
    // Each <ins> may only be pushed once; AdSense marks filled slots.
    if (!ins || ins.getAttribute("data-adsbygoogle-status")) return;
    try {
      window.adsbygoogle = window.adsbygoogle || [];
      if (nonPersonalized) window.adsbygoogle.requestNonPersonalizedAds = 1;
      window.adsbygoogle.push({});
    } catch (e) {
      console.warn("adsense", e);
    }
  }, [client, slot, nonPersonalized]);
  const test = location.hostname === "localhost" || location.hostname === "127.0.0.1" ? "on" : undefined;
  return (
    <ins
      ref={ref}
      className="adsbygoogle block"
      style={{ display: "block", width: "100%", height: placement === "game" ? 64 : undefined }}
      data-ad-client={client}
      data-ad-slot={slot}
      data-ad-format={placement === "game" ? "horizontal" : "auto"}
      data-full-width-responsive={placement === "game" ? "false" : "true"}
      data-adtest={test}
    />
  );
}

// ---- House promo ------------------------------------------------------------------

function HouseAd({ placement }: { placement: Placement }) {
  if (placement === "game") {
    return (
      <Link to="/shop" className="h-full flex items-center gap-3 px-3 bg-slate-900/80 border border-dashed border-slate-700 hover:border-emerald-500/60 text-xs text-slate-400">
        <Mascot size={40} className="shrink-0" />
        <span>
          <span className="text-slate-200">Tired of ads?</span> Go premium for private tables, 8 seats and house rules.
        </span>
      </Link>
    );
  }
  return (
    <Link to="/shop" className="relative block h-full p-4 bg-gradient-to-br from-emerald-900/40 to-slate-900 border border-emerald-500/30 hover:border-emerald-400/60 overflow-hidden">
      <Mascot size={96} className="absolute -right-3 -bottom-4 opacity-80" />
      <div className="text-xs uppercase tracking-wide text-emerald-300 mb-1">Monopsony Premium</div>
      <div className="font-semibold">No ads. Bigger tables. Your rules.</div>
      <p className="text-sm text-slate-400 mt-1">Private rooms, up to 8 players, house rules, every cosmetic slot and your full match history.</p>
      <div className="mt-3 inline-block px-3 py-1 rounded bg-emerald-500 text-slate-950 text-sm font-semibold">See plans</div>
    </Link>
  );
}

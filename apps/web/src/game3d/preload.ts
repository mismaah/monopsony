import { useEffect, useMemo, useState } from "react";
import * as THREE from "three";
import { useEnvironment, useGLTF, useTexture } from "@react-three/drei";
import { modelURL, tokenSkinFor, type Manifests } from "@monopsony/cosmetics";
import type { SeatInfo } from "@monopsony/protocol";
import { socket } from "@/api/ws";
import { useCosmetics } from "@/store/cosmetics";

/**
 * Preloading the table. The 3D scene pulls a handful of assets the moment it
 * mounts — the board emblem, an HDR environment off a CDN, a glTF model per
 * glTF-backed token — and each of them takes a while on a cold cache. The
 * server holds the deal until every player's client has reported in here, so
 * warm the very caches <Scene> reads: drei's loaders are all keyed by URL, so
 * a preload with the same arguments turns the scene's first frame into a
 * cache hit.
 *
 * Fonts are not covered: drei's <Text> fetches troika's default face itself
 * and gives us nothing to wait on.
 */

/** Must match what <Scene> renders, or this warms a cache nobody reads. */
export const ENVIRONMENT_PRESET = "city";
/** Must match <Board>'s CentreEmblem. */
export const BOARD_CENTRE_URL = "/brand/board-centre.png";

/** A loader hand-off (a .glb finishing, its textures starting) dips the queue to empty for a tick. */
const SETTLE_MS = 150;
/** Stop watching a download that is clearly not coming; the server's own grace is the real bound. */
const GIVE_UP_MS = 15_000;

interface Model {
  url: string;
  draco: boolean | undefined;
}

/** Every distinct glTF token at the table, in the form <TokenMesh> asks for it. */
function tokenModels(manifests: Manifests, seats: SeatInfo[]): Model[] {
  const byURL = new Map<string, Model>();
  seats.forEach((seat, i) => {
    const skin = tokenSkinFor(manifests, seat.loadout, i);
    const url = modelURL(skin.model);
    if (url && !byURL.has(url)) byURL.set(url, { url, draco: skin.model?.draco ? true : undefined });
  });
  return [...byURL.values()];
}

/**
 * Warm three's caches for everything the table needs and resolve once the
 * loaders fall idle. Resolves false if they are still busy after GIVE_UP_MS,
 * which is the caller's cue not to claim it is ready.
 */
export function preloadTableAssets(manifests: Manifests, seats: SeatInfo[], onProgress?: (done: number, total: number) => void): Promise<boolean> {
  const models = tokenModels(manifests, seats);
  return whileLoading(() => {
    useTexture.preload(BOARD_CENTRE_URL);
    useEnvironment.preload({ preset: ENVIRONMENT_PRESET });
    for (const m of models) useGLTF.preload(m.url, m.draco);
  }, onProgress);
}

type Watcher = (started: boolean) => void;

const watchers = new Set<Watcher>();
let instrumented = false;

/**
 * Report every load three starts and finishes to the watchers. The wrapper is
 * installed once and left in place: two preloads can overlap (StrictMode
 * mounts effects twice, and a player equipping a token restarts ours), and
 * wrappers that each restore "the previous" handler unhook each other.
 */
function instrument() {
  if (instrumented) return;
  instrumented = true;
  const mgr = THREE.DefaultLoadingManager;
  const itemStart = mgr.itemStart.bind(mgr);
  const itemEnd = mgr.itemEnd.bind(mgr);
  mgr.itemStart = (url: string) => {
    itemStart(url);
    watchers.forEach((w) => w(true));
  };
  // Loaders call itemEnd on failure too (after itemError), so a 404 cannot
  // wedge the queue — the scene falls back to its builtin shapes anyway.
  mgr.itemEnd = (url: string) => {
    itemEnd(url);
    watchers.forEach((w) => w(false));
  };
}

/**
 * Run `fire` and resolve once the loads it triggered have drained. Counting
 * items from the moment we start watching keeps the progress about this
 * preload: the manager's own totals are cumulative for the life of the page.
 */
function whileLoading(fire: () => void, onProgress?: (done: number, total: number) => void): Promise<boolean> {
  instrument();
  return new Promise<boolean>((resolve) => {
    let started = 0;
    let finished = 0;
    let settle: ReturnType<typeof setTimeout> | null = null;
    let settled = false;

    const finish = (ok: boolean) => {
      if (settled) return;
      settled = true;
      if (settle) clearTimeout(settle);
      clearTimeout(giveUp);
      watchers.delete(watch);
      resolve(ok);
    };
    const check = () => {
      onProgress?.(finished, started);
      if (settle) clearTimeout(settle);
      // Empty once is not empty for good: a .glb that just finished is about
      // to queue its textures. Only an idle stretch counts as done.
      if (started === finished) settle = setTimeout(() => finish(true), SETTLE_MS);
    };
    const watch: Watcher = (begun) => {
      if (begun) started++;
      else finished++;
      check();
    };
    const giveUp = setTimeout(() => finish(false), GIVE_UP_MS);
    watchers.add(watch);

    try {
      fire();
    } catch (e) {
      console.warn("table preload failed", e);
    }
    check(); // everything already cached: the loaders never stirred
  });
}

export interface TablePreload {
  /** Every asset is in three's caches. */
  ready: boolean;
  done: number;
  total: number;
}

/**
 * Preload the table for a lobby and tell the server once it is in, so the
 * host's start does not deal into scenes half the table cannot see yet.
 * Spectators (no seat) still preload; they just have nothing to report.
 */
export function useTablePreload(gameId: string | null, seats: SeatInfo[], mySeat: SeatInfo | undefined): TablePreload {
  const manifests = useCosmetics((s) => s.manifests);
  const [state, setState] = useState<TablePreload>({ ready: false, done: 0, total: 0 });

  // Seats arrive as a fresh array with every lobby broadcast; the models they
  // resolve to are what actually decides whether there is more to fetch.
  const models = useMemo(() => tokenModels(manifests, seats).map((m) => m.url).join("|"), [manifests, seats]);

  useEffect(() => {
    if (!gameId) return;
    let cancelled = false;
    void (async () => {
      // Without the manifests every seat looks like a builtin shape and the
      // glTF tokens would be missed.
      await useCosmetics.getState().load();
      const ok = await preloadTableAssets(useCosmetics.getState().manifests, seats, (done, total) => {
        if (!cancelled) setState((s) => ({ ...s, done, total }));
      });
      // Assets equipped later add to the set; never take "ready" back.
      if (!cancelled && ok) setState((s) => ({ ...s, ready: true }));
    })();
    return () => {
      cancelled = true;
    };
  }, [gameId, models]); // eslint-disable-line react-hooks/exhaustive-deps

  // Report in, and keep reporting until the server says it heard us: a frame
  // sent while the socket was down is simply dropped, and every reconnect
  // brings a fresh lobby to retry on.
  useEffect(() => {
    if (gameId && state.ready && mySeat && !mySeat.loaded) socket.assetsReady(gameId);
  }, [gameId, state.ready, mySeat]);

  return state;
}

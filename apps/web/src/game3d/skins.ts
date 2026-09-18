// Cosmetics are data: a loadout maps a slot to an item id, and the scene
// renders whatever the item's manifest describes. The maps below are the
// built-in looks; a manifest can pick one by `builtin`, override colours
// and PBR parameters, or point at a glTF model (bundled or uploaded).

export type TokenShape = "pawn" | "cone" | "sphere" | "ring" | "cube" | "gem";

/** Mirror of server/internal/cosmetics/manifest.go (validated there). */
export interface Manifest {
  builtin?: string;
  color?: string;
  model?: ModelRef;
  palette?: Record<string, string>;
  material?: { metalness?: number; roughness?: number; emissive?: string };
  preview?: string;
}

export interface ModelRef {
  url?: string;
  builtin?: string;
  scale?: number;
  offset?: [number, number, number];
  rotation?: [number, number, number];
  draco?: boolean;
}

export type Manifests = Record<string, Manifest>;

export interface TokenSkin {
  id: string;
  name: string;
  shape: TokenShape;
  color?: string; // undefined = use the seat colour
  metalness: number;
  roughness: number;
  emissive?: string;
  /** glTF model; the shape is the fallback while it loads (or if it fails). */
  model?: ModelRef;
}

export const TOKENS: Record<string, TokenSkin> = {
  "token.pawn": { id: "token.pawn", name: "Pawn", shape: "pawn", metalness: 0.2, roughness: 0.5 },
  "token.cone": { id: "token.cone", name: "Cone", shape: "cone", metalness: 0.2, roughness: 0.5 },
  "token.sphere": { id: "token.sphere", name: "Marble", shape: "sphere", metalness: 0.6, roughness: 0.2 },
  "token.ring": { id: "token.ring", name: "Ring", shape: "ring", metalness: 0.9, roughness: 0.15, color: "#e8c27a" },
  "token.cube": { id: "token.cube", name: "Block", shape: "cube", metalness: 0.1, roughness: 0.8 },
  "token.gem": { id: "token.gem", name: "Gem", shape: "gem", metalness: 0.3, roughness: 0.05, emissive: "#222" },
};

export interface BoardSkin {
  id: string;
  name: string;
  table: string; // felt / surface around the board
  tile: string;
  tileEdge: string;
  text: string;
  centre: string;
  house: string;
  hotel: string;
  mortgageTint: string;
}

export const BOARDS: Record<string, BoardSkin> = {
  "board.classic": {
    id: "board.classic",
    name: "Classic",
    table: "#173a2a",
    tile: "#e9e4d3",
    tileEdge: "#1f2937",
    text: "#1f2937",
    centre: "#cfe8d2",
    house: "#2e9e4f",
    hotel: "#c62828",
    mortgageTint: "#8a8577",
  },
  "board.midnight": {
    id: "board.midnight",
    name: "Midnight",
    table: "#0b1020",
    tile: "#1e2540",
    tileEdge: "#0b1020",
    text: "#dbe4ff",
    centre: "#141a33",
    house: "#4ade80",
    hotel: "#f43f5e",
    mortgageTint: "#3a3f55",
  },
};

export interface DiceSkin {
  id: string;
  name: string;
  body: string;
  pip: string;
}

export const DICE: Record<string, DiceSkin> = {
  "dice.ivory": { id: "dice.ivory", name: "Ivory", body: "#f5f1e6", pip: "#111827" },
  "dice.onyx": { id: "dice.onyx", name: "Onyx", body: "#111827", pip: "#f5f1e6" },
  "dice.ruby": { id: "dice.ruby", name: "Ruby", body: "#b91c1c", pip: "#fef2f2" },
};

export const DEFAULT_TOKENS = ["token.pawn", "token.cone", "token.sphere", "token.cube", "token.gem", "token.ring", "token.pawn", "token.cone"];

/** Resolve a token item id (or a seat default) through its manifest. */
export function resolveToken(manifests: Manifests, id: string | undefined, seatIndex: number): TokenSkin {
  const itemId = id ?? DEFAULT_TOKENS[seatIndex % DEFAULT_TOKENS.length];
  const m = manifests[itemId];
  const base = TOKENS[m?.builtin ?? itemId] ?? TOKENS["token.pawn"];
  const skin: TokenSkin = { ...base, id: itemId, name: base.name };
  if (m?.color) skin.color = m.color;
  if (m?.material?.metalness !== undefined) skin.metalness = m.material.metalness;
  if (m?.material?.roughness !== undefined) skin.roughness = m.material.roughness;
  if (m?.material?.emissive) skin.emissive = m.material.emissive;
  if (m?.model && (m.model.url || m.model.builtin)) skin.model = m.model;
  return skin;
}

export function tokenSkinFor(manifests: Manifests, loadout: Record<string, string> | undefined, seatIndex: number): TokenSkin {
  return resolveToken(manifests, loadout?.token, seatIndex);
}

/** Board skin: builtin base plus any palette overrides from the manifest. */
export function resolveBoard(manifests: Manifests, id: string | undefined): BoardSkin {
  const m = id ? manifests[id] : undefined;
  const base = BOARDS[m?.builtin ?? id ?? ""] ?? BOARDS["board.classic"];
  return { ...base, ...(m?.palette ?? {}), id: id ?? base.id } as BoardSkin;
}

export function resolveDice(manifests: Manifests, id: string | undefined): DiceSkin {
  const m = id ? manifests[id] : undefined;
  const base = DICE[m?.builtin ?? id ?? ""] ?? DICE["dice.ivory"];
  return { ...base, ...(m?.palette ?? {}), id: id ?? base.id } as DiceSkin;
}

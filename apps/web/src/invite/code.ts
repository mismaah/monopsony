import type { Lobby } from "@monopsony/protocol";

/** Where a shared invite lands: /join/CODE, handled by the SPA. */
export const JOIN_PATH = "/join";

/**
 * Accepts anything a player might paste — a full invite link, a code with or
 * without its dash, lower case — and returns the canonical `ABCD-EFGH` form
 * the server stores. Returns "" when there is nothing code-like in there.
 */
export function normalizeInviteCode(input: string): string {
  const trimmed = input.trim();
  const marker = `${JOIN_PATH}/`;
  const at = trimmed.lastIndexOf(marker);
  // A link that is not an invite link has no code in it, and squeezing its
  // letters into something code-shaped would only produce a confusing error.
  if (at < 0 && trimmed.includes("/")) return "";
  const chars = (at < 0 ? trimmed : trimmed.slice(at + marker.length))
    .toUpperCase()
    .replace(/[^A-Z2-7]/g, "") // the server mints codes in base32
    .slice(0, 8);
  return chars.length > 4 ? `${chars.slice(0, 4)}-${chars.slice(4)}` : chars;
}

/**
 * The link to hand a friend. Private tables travel as their invite code so the
 * link survives the game being re-created; public ones point straight at the
 * room, which anyone may join.
 */
export function inviteUrl(lobby: Pick<Lobby, "gameId" | "inviteCode">, origin = window.location.origin): string {
  return lobby.inviteCode ? `${origin}${JOIN_PATH}/${lobby.inviteCode}` : `${origin}/game/${lobby.gameId}`;
}

// ---- invites held across a sign-in ----------------------------------------------------

const PENDING_KEY = "monopsony.pendingInvite";

/**
 * A signed-out visitor following an invite has to authenticate first, and an
 * OAuth round trip drops the router state. Park the code in session storage so
 * the redirect after sign-in can still land them at the table.
 */
export function rememberInvite(code: string) {
  try {
    sessionStorage.setItem(PENDING_KEY, code);
  } catch {
    // Private browsing, or storage is full: the link still works, the OAuth
    // detour just ends at the lobby.
  }
}

export function pendingInvite(): string | null {
  try {
    return sessionStorage.getItem(PENDING_KEY);
  } catch {
    return null;
  }
}

export function clearPendingInvite() {
  try {
    sessionStorage.removeItem(PENDING_KEY);
  } catch {
    // ignore
  }
}

/** Where to send someone once they are signed in, invite or not. */
export function afterSignInPath(from?: string | null): string {
  if (from && from !== "/") return from;
  const code = pendingInvite();
  return code ? `${JOIN_PATH}/${code}` : "/lobby";
}

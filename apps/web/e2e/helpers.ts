import { expect, type APIRequestContext, type Page } from "@playwright/test";

const API = "http://localhost:8091";

/** Sign in as a guest through the UI and land in the lobby. */
export async function guest(page: Page, name: string) {
  // Surface client crashes in the test output instead of a blank screenshot.
  page.on("pageerror", (e) => console.log(`[pageerror] ${e.message}`));
  await page.goto("/");
  await page.getByRole("button", { name: "Play as guest" }).click();
  await page.getByPlaceholder("Display name").fill(name);
  await page.getByRole("button", { name: "Jump in" }).click();
  await expect(page).toHaveURL(/\/lobby/);
}

/** Create a table from the lobby page and return the game id. */
export async function createTable(page: Page, name: string) {
  await page.getByPlaceholder("Table name").fill(name);
  await page.getByRole("button", { name: "Create table" }).click();
  await expect(page).toHaveURL(/\/game\//);
  await expect(page.getByRole("heading", { name })).toBeVisible();
  return page.url().split("/game/")[1];
}

/** Add n bots and start; resolves once the 3D scene is up. */
export async function startWithBots(page: Page, n: number) {
  for (let i = 0; i < n; i++) await page.getByRole("button", { name: "+ balanced bot" }).click();
  await page.getByRole("button", { name: "Start game" }).click();
  await expect(page.locator("canvas").first()).toBeVisible();
}

/** Read a slice of the live client store (exposed in dev builds). */
export function store<T>(page: Page, fn: (s: GameState) => T): Promise<T> {
  // The function runs in the browser: serialise it and rebuild the getter.
  return page.evaluate((src) => {
    const w = window as unknown as { __monopsony: { useGame: { getState(): unknown } } };
    const f = new Function("s", `return (${src})(s)`) as (s: unknown) => T;
    return f(w.__monopsony.useGame.getState());
  }, fn.toString());
}

/** The shape the specs read from the game store. */
export interface GameState {
  gameId: string | null;
  seats: { playerId: string; name: string; loadout?: Record<string, string> }[];
  state: { seq: number; players: { id: string; name: string; position: number; cash: number }[]; turn: { phase: string; playerIdx: number } } | null;
  config: { spaces: { name: string }[]; rules: { startCash: number } } | null;
  legal: { type: string }[];
}

/** Register (or log in) an admin against the API and return a bearer token. */
export async function adminToken(request: APIRequestContext) {
  const creds = { email: "admin@e2e.test", password: "e2e-admin-pass!", name: "Admin" };
  let res = await request.post(`${API}/api/auth/register`, { data: creds });
  if (res.status() === 409) res = await request.post(`${API}/api/auth/login`, { data: creds });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).accessToken as string;
}

export { API };

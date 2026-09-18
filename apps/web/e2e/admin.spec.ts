import { test, expect } from "@playwright/test";
import { API, adminToken, createTable, guest, startWithBots, store } from "./helpers";

test("publishing a config affects new games but not games in progress", async ({ page, request, browser }) => {
  // A game started on the current published config.
  await guest(page, "Config Tester");
  await createTable(page, "Old rules");
  await startWithBots(page, 1);
  const before = await store(page, (s) => ({ name: s.config!.spaces[39].name, cash: s.config!.rules.startCash }));
  expect(before).toEqual({ name: "Captain's Reach", cash: 1500 });

  // Admin publishes a new version: renamed last street, bigger start cash.
  const token = await adminToken(request);
  const auth = { Authorization: `Bearer ${token}` };
  const list = await (await request.get(`${API}/admin/api/configs`, { headers: auth })).json();
  const published = list.configs.find((c: { published: boolean }) => c.published);
  const full = await (await request.get(`${API}/admin/api/configs/${published.id}`, { headers: auth })).json();
  const config = full.config.config;
  config.spaces[39].name = "Monopsony Plaza";
  config.rules.startCash = 1600;
  const created = await request.post(`${API}/admin/api/configs`, { headers: auth, data: { name: "e2e theme", config, publish: true } });
  expect(created.status(), await created.text()).toBe(201);

  // The running game is pinned to its version.
  await page.reload();
  await expect(page.locator("canvas").first()).toBeVisible();
  await expect.poll(() => store(page, (s) => s.config?.spaces[39].name ?? null)).toBe("Captain's Reach");

  // A new table picks up the published version. The free tier allows one
  // active game per user, so a second user (the admin) creates it.
  const adminCtx = await browser.newContext();
  const admin = await adminCtx.newPage();
  await admin.goto("/");
  await admin.getByRole("button", { name: "login" }).click();
  await admin.getByPlaceholder("Email").fill("admin@e2e.test");
  await admin.getByPlaceholder("Password").fill("e2e-admin-pass!");
  await admin.getByRole("button", { name: "Sign in" }).click();
  await expect(admin).toHaveURL(/\/lobby/);
  await createTable(admin, "New rules");
  await startWithBots(admin, 1);
  const after = await store(admin, (s) => ({ name: s.config!.spaces[39].name, cash: s.config!.rules.startCash, playerCash: s.state!.players[0].cash }));
  expect(after).toEqual({ name: "Monopsony Plaza", cash: 1600, playerCash: 1600 });
  await adminCtx.close();
});

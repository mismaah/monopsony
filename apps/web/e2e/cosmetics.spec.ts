import { test, expect } from "@playwright/test";
import { createTable, guest, store } from "./helpers";

test("equipping a cosmetic shows up on another player's client", async ({ browser }) => {
  const ctxA = await browser.newContext();
  const ctxB = await browser.newContext();
  const a = await ctxA.newPage();
  const b = await ctxB.newPage();
  const modelErrors: string[] = [];
  for (const p of [a, b]) p.on("console", (m) => m.text().includes("token model failed") && modelErrors.push(m.text()));

  await guest(a, "Alice");
  const id = await createTable(a, "Skins table");

  // Bob joins the public table from the lobby list.
  await guest(b, "Bob");
  await b.getByRole("listitem").filter({ hasText: "Skins table" }).getByRole("button", { name: "Join" }).click();
  await expect(b).toHaveURL(new RegExp(`/game/${id}`));
  await expect(b.getByText("Alice")).toBeVisible();

  // Alice equips a different token in the shop. The glTF-backed Top Hat is
  // in the catalog with a live 3D preview.
  await a.goto("/shop");
  await expect(a.getByTestId("token-preview-token.tophat")).toBeVisible();
  const cone = a.locator("div", { has: a.getByText("Cone", { exact: true }) }).filter({ has: a.getByRole("button", { name: "Equip" }) }).last();
  await cone.getByRole("button", { name: "Equip" }).click();
  await expect(cone.getByRole("button", { name: "Equipped" })).toBeVisible();

  // Back at the table, the loadout is refreshed on (re)subscribe and
  // broadcast with the seats, so Bob's client sees it.
  await a.goto(`/game/${id}`);
  await expect(a.getByText("Bob")).toBeVisible();
  await expect
    .poll(async () => {
      const seats = await store(b, (s) => s.seats);
      return seats.find((s) => s.name === "Alice")?.loadout?.token ?? null;
    })
    .toBe("token.cone");

  // Start with a bot so the scene renders both loadouts.
  await a.getByRole("button", { name: "+ balanced bot" }).click();
  await a.getByRole("button", { name: "Start game" }).click();
  await expect(b.locator("canvas").first()).toBeVisible();
  await expect(a.locator("canvas").first()).toBeVisible();
  const seatsB = await store(b, (s) => s.seats);
  expect(seatsB.find((s) => s.name === "Alice")?.loadout?.token).toBe("token.cone");
  expect(modelErrors).toEqual([]);

  await ctxA.close();
  await ctxB.close();
});

test("the collection page lists owned items and switches the loadout", async ({ page }) => {
  await guest(page, "Carol");
  await page.getByRole("link", { name: "Collection" }).click();
  await expect(page).toHaveURL(/\/collection$/);

  // Free items are owned by default; paid ones are not in the collection.
  // Every slot gets a live 3D preview, all drawn on one shared canvas.
  await expect(page.getByTestId("board-preview-board.classic")).toBeVisible();
  await expect(page.getByTestId("dice-preview-dice.ivory")).toBeVisible();
  await expect(page.locator("canvas")).toHaveCount(1);
  const cone = page.locator("div", { has: page.getByText("Cone", { exact: true }) }).filter({ has: page.getByRole("button", { name: /Equip/ }) }).last();
  await expect(cone).toBeVisible();
  await expect(page.getByRole("button", { name: "Buy" })).toHaveCount(0);

  await cone.getByRole("button", { name: "Equip" }).click();
  await expect(cone.getByRole("button", { name: "Equipped" })).toBeVisible();

  // "Use default" clears the slot again.
  await page.getByRole("button", { name: "Use default" }).first().click();
  await expect(cone.getByRole("button", { name: "Equip", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Use default" })).toHaveCount(0);
});

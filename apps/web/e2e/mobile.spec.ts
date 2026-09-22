import { test, expect, type Page } from "@playwright/test";
import { createTable, guest, startWithBots } from "./helpers";

/**
 * Phone layout. The desktop HUD floats fixed-width panels in the four
 * corners; below `sm` they stack into a top strip and a bottom strip
 * instead, so what this guards is that nothing lands off screen or on top
 * of the action bar at phone sizes.
 *
 * iPhone-13 metrics on chromium — the devices[] preset would force webkit.
 */
test.use({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true });

/** Page-level horizontal overflow: the classic "why does it scroll sideways". */
async function overflow(page: Page) {
  return page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
}

/** True when the element sits entirely inside the viewport. */
async function onScreen(page: Page, selector: string) {
  const box = await page.locator(selector).first().boundingBox();
  const size = page.viewportSize()!;
  return !!box && box.x >= 0 && box.y >= 0 && box.x + box.width <= size.width + 1 && box.y + box.height <= size.height + 1;
}

test("phone: lobby and table fit the screen", async ({ page }) => {
  await guest(page, "Pocket Player");
  expect(await overflow(page)).toBeLessThanOrEqual(0);

  // The nav collapses behind a menu button rather than crowding the header.
  // (Scoped to the header: the house ad also links to "…Bigger tables…".)
  const tables = page.locator("header").getByRole("link", { name: "Tables", exact: true });
  await expect(tables).toBeHidden();
  await page.getByRole("button", { name: "Menu" }).click();
  await expect(tables).toBeVisible();
  await tables.click();

  await createTable(page, "Pocket table");
  expect(await overflow(page)).toBeLessThanOrEqual(0);
});

test("phone: the in-game HUD stacks instead of overlapping", async ({ page }) => {
  await guest(page, "Pocket Host");
  await createTable(page, "Pocket game");
  await startWithBots(page, 3);

  // Every player is on the compact strip, and the action bar is reachable.
  for (const name of ["You", "Ada (bot)"]) await expect(page.getByText(name, { exact: true }).first()).toBeVisible();
  const roll = page.getByRole("button", { name: "Roll dice" });
  await expect(roll).toBeVisible();
  expect(await onScreen(page, "button:has-text('Roll dice')")).toBe(true);
  expect(await overflow(page)).toBeLessThanOrEqual(0);

  // A deed opens in the bottom strip, above the action bar, and the log
  // collapses to one line until it is asked for.
  await page.evaluate(() => (window as unknown as { __monopsony: { useGame: { getState(): { select(i: number | null): void } } } }).__monopsony.useGame.getState().select(1));
  await expect(page.getByText("Mortgage value")).toBeVisible();
  expect(await onScreen(page, "button:has-text('Roll dice')")).toBe(true);
  expect(await overflow(page)).toBeLessThanOrEqual(0);

  const log = page.locator("button[aria-expanded]").last();
  await log.click();
  await expect(page.getByPlaceholder("Say something…")).toBeVisible();
  expect(await overflow(page)).toBeLessThanOrEqual(0);

  // Turned sideways the same strips apply, and the board still has room.
  await page.setViewportSize({ width: 844, height: 390 });
  await expect(roll).toBeVisible();
  expect(await onScreen(page, "button:has-text('Roll dice')")).toBe(true);
  expect(await overflow(page)).toBeLessThanOrEqual(0);
});

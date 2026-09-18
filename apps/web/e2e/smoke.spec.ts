import { test, expect } from "@playwright/test";
import { createTable, guest, startWithBots, store } from "./helpers";

test.describe("smoke", () => {
  test("guest → create table → bots → roll → buy/decline → end turn", async ({ page }) => {
    await guest(page, "Smoke Host");
    await createTable(page, "Smoke table");
    await startWithBots(page, 3);

    // The host is seat 0, so the first decision is theirs.
    const roll = page.getByRole("button", { name: "Roll dice" });
    await expect(roll).toBeEnabled();
    const before = await store(page, (s) => s.state!.seq);
    await roll.click();

    // After the roll one of these is legal: buy/auction the space, or end
    // the turn (tax, card, doubles...). Bots then play through their turns.
    const buy = page.getByRole("button", { name: "Buy", exact: true });
    const next = page.getByRole("button", { name: /End turn|Continue/ });
    await expect(buy.or(next)).toBeVisible();
    if (await buy.isVisible()) {
      await buy.click();
      await expect(next).toBeVisible();
    }
    await next.click();

    await expect.poll(() => store(page, (s) => s.state!.seq)).toBeGreaterThan(before);
    const me = await store(page, (s) => s.state!.players[0]);
    expect(me.name).toBe("Smoke Host");
    // Players rail lists everyone; the 3D canvas is live.
    for (const name of ["Smoke Host", "(bot)"]) await expect(page.getByText(name).first()).toBeVisible();
    await expect(page.locator("canvas").first()).toBeVisible();

    // Free tier: the ad slot renders with the configured (house) provider.
    const ad = page.getByTestId("ad-slot");
    await expect(ad).toBeVisible();
    await expect(ad).toHaveAttribute("data-provider", "house");

    await page.screenshot({ path: "test-results/smoke-board.png" });
  });

  test("reconnect: a reload mid-game restores the same state", async ({ page }) => {
    await guest(page, "Reloader");
    const id = await createTable(page, "Reload table");
    await startWithBots(page, 2);
    await page.getByRole("button", { name: "Roll dice" }).click();
    await expect(page.getByRole("button", { name: /Buy|End turn|Continue/ }).first()).toBeVisible();
    const snapshot = await store(page, (s) => ({ seq: s.state!.seq, positions: s.state!.players.map((p) => p.position), cash: s.state!.players.map((p) => p.cash) }));

    await page.reload();
    await expect(page).toHaveURL(new RegExp(`/game/${id}`));
    await expect(page.locator("canvas").first()).toBeVisible();
    await expect.poll(() => store(page, (s) => s.state?.seq ?? -1)).toBeGreaterThanOrEqual(snapshot.seq);
    const after = await store(page, (s) => ({ seq: s.state!.seq, positions: s.state!.players.map((p) => p.position), cash: s.state!.players.map((p) => p.cash) }));
    // Nobody else could act while we were waiting on our own decision.
    expect(after).toEqual(snapshot);
    // And the decision is still ours to make.
    await expect(page.getByRole("button", { name: /Buy|End turn|Continue/ }).first()).toBeVisible();
  });
});

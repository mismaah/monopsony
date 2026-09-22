import { test, expect, type Page } from "@playwright/test";
import { createTable, guest } from "./helpers";

/** Register through the front door. Admin emails resolve to premium. */
async function register(page: Page, email: string, name: string) {
  page.on("pageerror", (e) => console.log(`[pageerror] ${e.message}`));
  await page.getByRole("button", { name: "register", exact: true }).click();
  await page.getByPlaceholder("Display name").fill(name);
  await page.getByPlaceholder("Email").fill(email);
  await page.getByPlaceholder("Password").fill("e2e-invite-pass!");
  await page.getByRole("button", { name: "Create account" }).click();
}

test.describe("invite", () => {
  test("a public table is shared by its room link", async ({ page, browser, baseURL }) => {
    await guest(page, "Invite Host");
    const id = await createTable(page, "Public invite table");

    const link = page.getByLabel("Invite link");
    await expect(link).toHaveValue(`${baseURL}/game/${id}`);
    // The QR names its destination, so it doubles as the assertion that the
    // symbol is rendered for the same link.
    await expect(page.getByRole("img", { name: `QR code linking to ${baseURL}/game/${id}` })).toBeVisible();

    const friend = await (await browser.newContext()).newPage();
    await friend.goto(await link.inputValue());
    // The room needs an account, so the link lands at the front door and
    // comes back once there is one.
    await expect(friend).toHaveURL(/\/$/);
    await friend.getByPlaceholder("Display name").fill("Invited Friend");
    await friend.getByRole("button", { name: "Jump in" }).click();
    await expect(friend).toHaveURL(new RegExp(`/game/${id}$`));
    await friend.getByRole("button", { name: "Take a seat" }).click();

    await expect(page.getByText("Invited Friend")).toBeVisible();
  });

  test("a private table's link carries its invite code and seats the guest", async ({ page, browser }) => {
    await page.goto("/");
    await register(page, "host@e2e.test", "Invite Host 2");
    await expect(page).toHaveURL(/\/lobby/);

    await page.getByPlaceholder("Table name").fill("Private invite table");
    await page.getByLabel("Private (invite code)").check();
    await page.getByRole("button", { name: "Create table" }).click();
    await expect(page).toHaveURL(/\/game\//);
    const id = page.url().split("/game/")[1];

    const link = await page.getByLabel("Invite link").inputValue();
    expect(link).toMatch(/\/join\/[A-Z2-7]{4}-[A-Z2-7]{4}$/);

    const friend = await (await browser.newContext()).newPage();
    await friend.goto(link);
    await expect(friend).toHaveURL(/\/$/);
    await register(friend, "friend@e2e.test", "Invited Friend 2");

    // Following the link seats them: no lobby detour, no code to type.
    await expect(friend).toHaveURL(new RegExp(`/game/${id}$`));
    await expect(page.getByText("Invited Friend 2")).toBeVisible();
  });
});

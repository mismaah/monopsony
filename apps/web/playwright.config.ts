import { defineConfig, devices } from "@playwright/test";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

/**
 * End-to-end smoke suite. Playwright boots a throwaway Go server (in-memory
 * store, fast bots, generous rate limits) on :8091 and the Vite dev server
 * on :5199, so it never collides with a dev stack on the default ports.
 * Run with `npm run e2e -w @monopsony/web` (or `npm run e2e` from the
 * app folder); `npm run e2e:ui` opens the inspector.
 */
const API_PORT = 8091;
const WEB_PORT = 5199;
const assetDir = mkdtempSync(join(tmpdir(), "monopsony-e2e-assets-"));

export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  outputDir: "test-results",
  use: {
    baseURL: `http://localhost:${WEB_PORT}`,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "off",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: [
    {
      command: "go run ./cmd/server",
      cwd: "../../server",
      url: `http://localhost:${API_PORT}/healthz`,
      reuseExistingServer: false,
      timeout: 180_000,
      env: {
        MONOPSONY_ADDR: `127.0.0.1:${API_PORT}`,
        // The server also reads the repo-root .env (godotenv never overrides
        // a set variable), so pin the throwaway stores explicitly.
        MONOPSONY_DATABASE_URL: "",
        MONOPSONY_REDIS_URL: "",
        MONOPSONY_METRICS_ADDR: "off",
        MONOPSONY_CORS_ORIGINS: `http://localhost:${WEB_PORT}`,
        MONOPSONY_PUBLIC_URL: `http://localhost:${WEB_PORT}`,
        MONOPSONY_JWT_SECRET: "e2e-secret",
        MONOPSONY_ADMIN_EMAILS: "admin@e2e.test",
        MONOPSONY_BOT_DELAY_MS: "60",
        // Clients still report their assets loaded; this only caps how long
        // a start waits for one that cannot (no CDN in CI, say).
        MONOPSONY_ASSET_GRACE_MS: "3000",
        MONOPSONY_ASSET_DIR: assetDir,
        MONOPSONY_ADS_PROVIDER: "house",
        MONOPSONY_RATE_AUTH: "600/100",
        MONOPSONY_RATE_API: "6000/1000",
        MONOPSONY_RATE_WS: "600/100",
        MONOPSONY_LOG_LEVEL: "warn",
      },
    },
    {
      command: "npx vite --port " + WEB_PORT,
      url: `http://localhost:${WEB_PORT}`,
      reuseExistingServer: false,
      timeout: 120_000,
      env: { MONOPSONY_API: `http://localhost:${API_PORT}`, PORT: String(WEB_PORT) },
    },
  ],
});

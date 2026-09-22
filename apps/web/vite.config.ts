import { defineConfig } from "vitest/config";
import { fileURLToPath, URL } from "node:url";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// The Go server runs on :8080 in dev; the SPA proxies API and WS calls to it
// so cookies and origins behave as in production (single origin). The e2e
// suite points both at spare ports via MONOPSONY_API / PORT.
const api = process.env.MONOPSONY_API ?? "http://localhost:8080";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  server: {
    port: Number(process.env.PORT ?? 5173),
    strictPort: true,
    proxy: {
      "/api": api,
      "/media": api,
      "/ws": { target: api.replace(/^http/, "ws"), ws: true },
    },
  },
  assetsInclude: ["**/*.glb"],
  // Unit tests live beside the source; e2e/ is Playwright's and throws if
  // vitest collects it.
  test: { include: ["src/**/*.{test,spec}.{ts,tsx}"] },
  build: { outDir: "dist", sourcemap: true },
});

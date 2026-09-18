import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";

// ADMIN_BASE=/admin/ for the single-image deploy (served under /admin/ by the Go server).
export default defineConfig({
  base: process.env.ADMIN_BASE ?? "/",
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  server: {
    port: 5174,
    proxy: {
      "/api": "http://localhost:8080",
      "/admin/api": "http://localhost:8080",
    },
  },
  build: { outDir: "dist" },
});

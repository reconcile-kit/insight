import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";

// In dev the Go backend (e.g. from the e2e-tests stand, `make insight`) serves
// both /insight and the proxied /api routes.
const backend = process.env.INSIGHT_BACKEND || "http://localhost:57182";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") },
  },
  server: {
    proxy: {
      "/insight": backend,
      "/api": backend,
    },
  },
  build: {
    outDir: "web/dist",
    emptyOutDir: true,
    chunkSizeWarningLimit: 1200,
  },
});

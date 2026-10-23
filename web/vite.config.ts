import { defineConfig } from "vitest/config";

export default defineConfig({
  build: {
    outDir: "dist",
    // dist/ is embedded by web/embed.go; the npm build script restores dist/.gitkeep.
    emptyOutDir: true,
    sourcemap: false,
    // One bundle by design: three.js dominates its size.
    chunkSizeWarningLimit: 1500,
  },
  // `make dev`: the backend runs on 127.0.0.1:8080 with
  // --allow-origin http://localhost:5173; the browser talks only to Vite.
  server: {
    host: "localhost",
    port: 5173,
    strictPort: true,
    proxy: {
      "/auth": "http://127.0.0.1:8080",
      "/api": "http://127.0.0.1:8080",
      "/ws": { target: "ws://127.0.0.1:8080", ws: true },
    },
  },
  test: {
    include: ["test/**/*.test.ts"],
    environment: "node",
  },
});

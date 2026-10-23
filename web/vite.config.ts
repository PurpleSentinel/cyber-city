import { defineConfig } from "vitest/config";

export default defineConfig({
  build: {
    outDir: "dist",
    // dist/ is embedded by web/embed.go; the Makefile restores dist/.gitkeep after a build.
    emptyOutDir: true,
    sourcemap: false,
    // One bundle by design: three.js dominates its size.
    chunkSizeWarningLimit: 1500,
  },
  test: {
    include: ["test/**/*.test.ts"],
    environment: "node",
  },
});

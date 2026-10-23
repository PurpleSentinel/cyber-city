import { defineConfig } from "@playwright/test";

// End-to-end suite (docs/TESTING.md "End-to-end"). The global setup starts
// bin/cybercity replaying testdata/pcap/demo.pcap; tests log in through the
// printed URL. Headless Chromium uses SwiftShader (software WebGL).
export default defineConfig({
  testDir: "e2e",
  timeout: 90_000,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  globalSetup: "./e2e/global-setup.ts",
  globalTeardown: "./e2e/global-teardown.ts",
  use: {
    viewport: { width: 1280, height: 720 },
    deviceScaleFactor: 1,
    browserName: "chromium",
  },
});

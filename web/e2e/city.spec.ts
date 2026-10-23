// docs/TESTING.md "End-to-end" steps 2–5: the city comes alive from the demo
// replay, under software rendering, with no CSP violations or errors.
import { expect, test } from "@playwright/test";
import { join } from "node:path";
import { ARTIFACTS, authURL, waitReady, watchProblems } from "./util";

test("the city renders from the demo replay", async ({ page }) => {
  const problems = await watchProblems(page);
  await page.goto(authURL("/?debug=1"));
  await waitReady(page);
  await expect(page).not.toHaveURL(/token=/); // token not left in the address bar

  await expect
    .poll(async () => page.evaluate(() => window.__cybercity!.stats()), { timeout: 30_000 })
    .toMatchObject({ lit: true, wireframe: false });
  await expect.poll(() => page.evaluate(() => window.__cybercity!.stats().buildings), { timeout: 30_000 }).toBeGreaterThanOrEqual(30);
  await expect.poll(() => page.evaluate(() => window.__cybercity!.stats().highways), { timeout: 30_000 }).toBeGreaterThanOrEqual(5);
  await expect.poll(() => page.evaluate(() => window.__cybercity!.stats().particles), { timeout: 30_000 }).toBeGreaterThan(0);
  // The demo scan fires 60 s into the capture: 15 s at 4x replay.
  await expect
    .poll(() => page.evaluate(() => window.__cybercity!.lastEvents().map((e) => e.kind)), { timeout: 25_000 })
    .toContain("port_scan");

  // Headless Chromium renders with SwiftShader: the warning shows and quality drops.
  const stats = await page.evaluate(() => window.__cybercity!.stats());
  if (stats.software) {
    await expect(page.locator("#warning")).toBeVisible();
    expect(stats.quality).toBe("low");
  }
  await expect(page.locator("#status")).toHaveText("Live");

  await page.screenshot({ path: join(ARTIFACTS, "city.png") });
  expect(problems.filter((p) => !p.includes("GPU stall"))).toEqual([]);
});

test("pausing drops ticks and resuming resyncs", async ({ page }) => {
  await page.goto(authURL("/?debug=1"));
  await waitReady(page);
  await expect(page.locator("#status")).toHaveText("Live", { timeout: 30_000 });
  await page.keyboard.press("Space");
  await expect(page.locator("#status")).toHaveText("Paused");
  await page.keyboard.press("Space");
  await expect(page.locator("#status")).toHaveText("Live", { timeout: 15_000 });
});
